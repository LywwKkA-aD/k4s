package k8s

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// ErrorKind is what went wrong with an API call, in terms the user can act
// on.
type ErrorKind int

// The kinds Classify tells apart.
const (
	// KindNone is no error at all.
	KindNone ErrorKind = iota
	// KindOther is any error the other kinds do not cover.
	KindOther
	// KindForbidden is RBAC refusing the request (403).
	KindForbidden
	// KindUnauthorized is the API rejecting the credentials (401), or a
	// credential plugin that failed to produce any.
	KindUnauthorized
	// KindLogin is a credential plugin that needs a terminal to log in
	// while k4s owns it (client-go's StdinUnavailable).
	KindLogin
	// KindNotFound is a 404.
	KindNotFound
	// KindTimeout is an API that did not answer in time.
	KindTimeout
	// KindUnreachable is a refused connection, an unknown host or no
	// route to the API server.
	KindUnreachable
	// KindThrottled is the API asking for fewer requests (429).
	KindThrottled
	// KindUnavailable is a 503: an API, often an aggregated one such as
	// metrics.k8s.io, that is registered but not serving.
	KindUnavailable
)

var kindNames = []string{
	"none", "other", "forbidden", "unauthorized", "login", "not found",
	"timeout", "unreachable", "throttled", "unavailable",
}

func (k ErrorKind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// Diagnosis is an API error classified for the status row and the error
// bodies: Short says what went wrong on one line and Hint what to run or do
// next, "" when there is nothing to suggest. Both are inert text: escape
// sequences, URL queries and URL credentials are gone.
type Diagnosis struct {
	Kind  ErrorKind
	Short string
	Hint  string

	// Verb, Resource and Namespace name a Forbidden request as `kubectl
	// auth can-i` does; AllNamespaces is set for a namespaced resource
	// asked across every namespace. They are empty for other kinds.
	Verb          string
	Resource      string
	Namespace     string
	AllNamespaces bool
}

// Text is Short, followed by " · " and Hint when there is one.
func (d Diagnosis) Text() string {
	if d.Hint == "" {
		return d.Short
	}
	return d.Short + " · " + d.Hint
}

// k3sKubeconfig exists only on a k3s server, which serves its API on
// 127.0.0.1:6443.
const k3sKubeconfig = "/etc/rancher/k3s/k3s.yaml"

// Classify sorts err into an ErrorKind and words it for the user.
func Classify(err error) Diagnosis {
	return classify(err, onK3sServer)
}

func onK3sServer() bool {
	_, err := os.Stat(k3sKubeconfig)
	return err == nil
}

const (
	retryHint     = "r retries"
	terminalLogin = "log in with kubectl in a terminal, then press r"
)

func classify(err error, k3sServer func() bool) Diagnosis {
	if err == nil {
		return Diagnosis{}
	}
	text := err.Error()
	if d, ok := credentialProblem(text); ok {
		return d
	}
	switch {
	case apierrors.IsUnauthorized(err):
		return Diagnosis{Kind: KindUnauthorized, Short: "unauthorized: credentials expired or revoked", Hint: "log in again, then press r"}
	case apierrors.IsForbidden(err):
		return classifyForbidden(err)
	case apierrors.IsNotFound(err):
		return notFound(err)
	case apierrors.IsTooManyRequests(err):
		return Diagnosis{Kind: KindThrottled, Short: "API throttled: too many requests (429)", Hint: "k4s slows down; r retries"}
	case apierrors.IsServiceUnavailable(err):
		return Diagnosis{Kind: KindUnavailable, Short: "API unavailable (503)", Hint: retryHint}
	case unreachable(err):
		return unreachableAt(apiHost(err), k3sServer)
	case timedOut(err):
		return Diagnosis{Kind: KindTimeout, Short: "API slow: " + slowCall(err), Hint: retryHint}
	}
	return Diagnosis{Kind: KindOther, Short: redact(text)}
}

var (
	pluginFailed  = regexp.MustCompile(`exec: executable (\S+) failed with exit code (-?\d+)`)
	pluginMissing = regexp.MustCompile(`exec: executable (\S+) not found`)
	stdinMissing  = regexp.MustCompile(`standard input is (?:unavailable|not a terminal)(?:: (.*))?`)
)

// credentialProblem reads client-go's exec credential plugin errors. They
// are formatted with %v, so only their text is left to go on.
func credentialProblem(text string) (Diagnosis, bool) {
	if m := stdinMissing.FindStringSubmatch(firstLine(text)); m != nil {
		hint := terminalLogin
		if m[1] != "" {
			hint = sanitize.Line(m[1])
		}
		return Diagnosis{Kind: KindLogin, Short: "login required: the credential plugin needs a terminal", Hint: hint}, true
	}
	_, rest, ok := strings.Cut(firstLine(text), "getting credentials: ")
	if !ok {
		return Diagnosis{}, false
	}
	d := Diagnosis{Kind: KindUnauthorized, Short: "credentials: " + redact(rest), Hint: terminalLogin}
	if m := pluginFailed.FindStringSubmatch(rest); m != nil {
		d.Short = "credentials: plugin " + pluginName(m[1]) + " failed (exit " + m[2] + ")"
	} else if m := pluginMissing.FindStringSubmatch(rest); m != nil {
		d.Short = "credentials: plugin " + pluginName(m[1]) + " not found"
		d.Hint = "install it or fix users[].user.exec.command in the kubeconfig"
	}
	return d, true
}

func pluginName(path string) string {
	return sanitize.Line(filepath.Base(path))
}

func notFound(err error) Diagnosis {
	msg := redact(apiMessage(err))
	if !strings.Contains(msg, "not found") {
		msg = "not found: " + msg
	}
	return Diagnosis{Kind: KindNotFound, Short: msg}
}

// apiMessage is the message of the API status inside err, else its text.
func apiMessage(err error) string {
	var status apierrors.APIStatus
	if errors.As(err, &status) && status.Status().Message != "" {
		return status.Status().Message
	}
	return err.Error()
}

func timedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// slowCall names the call that timed out when it is known.
func slowCall(err error) string {
	var call *CallError
	if errors.As(err, &call) {
		return call.Verb + " " + sanitize.Line(call.Resource) + " timed out"
	}
	return "request timed out"
}

// unreachable reports a dial that found nothing to talk to: a refused
// connection, no route to the host or its network, or a name DNS does not
// know. A dial that timed out is not one: a black-holed address and a slow
// API look the same from here, so timedOut has it.
func unreachable(err error) bool {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return dns.IsNotFound
	}
	return slices.ContainsFunc(unreachableErrnos, func(target error) bool { return errors.Is(err, target) })
}

// apiHost is the host:port the failed request went to, "" when err does
// not say.
func apiHost(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		if u, perr := url.Parse(ue.URL); perr == nil && u.Host != "" {
			return sanitize.Line(u.Host)
		}
	}
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return sanitize.Line(dns.Name)
	}
	var op *net.OpError
	if errors.As(err, &op) && op.Addr != nil {
		return op.Addr.String()
	}
	return ""
}

// k3sLoopback are the addresses a k3s server writes into its own
// k3s.yaml; elsewhere they point at nothing.
var k3sLoopback = map[string]bool{"127.0.0.1:6443": true, "localhost:6443": true, "[::1]:6443": true}

func unreachableAt(host string, k3sServer func() bool) Diagnosis {
	d := Diagnosis{Kind: KindUnreachable, Short: "API unreachable"}
	if host == "" {
		return d
	}
	d.Short += ": " + host
	switch {
	case !k3sLoopback[host]:
	case k3sServer():
		d.Hint = "is k3s running? sudo systemctl status k3s"
	default:
		d.Hint = host + " is the k3s server's own address: in a k3s.yaml copied to this machine, set server: to https://<server-ip>:6443"
	}
	return d
}

var (
	urlCredentials = regexp.MustCompile(`(https?://)[^\s"'/@]*@`)
	urlQuery       = regexp.MustCompile(`(https?://[^\s"'?#]*)[?#][^\s"']*`)
)

// redact is the first line of text made inert, with URL credentials and
// query strings cut: a token can ride in either.
func redact(text string) string {
	s := sanitize.Line(firstLine(text))
	s = urlCredentials.ReplaceAllString(s, "$1")
	return urlQuery.ReplaceAllString(s, "$1")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
