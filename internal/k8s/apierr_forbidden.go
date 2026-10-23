package k8s

import (
	"errors"
	"regexp"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// The apiserver words a denial as `User "u" cannot <verb> resource "<r>" in
// API group "<g>" in the namespace "<ns>"` (or `at the cluster scope`), or
// `User "u" cannot <verb> path "<p>"` for a non-resource URL. StatusDetails
// carries the resource and group but never the verb or namespace.
var (
	deniedResource = regexp.MustCompile(`cannot (\S+) resource "([^"]+)" in API group "([^"]*)"(?: in the namespace "([^"]+)"| (at the cluster scope))?`)
	deniedPath     = regexp.MustCompile(`cannot (\S+) path "([^"]+)"`)
)

// denial is what a Forbidden refused, as far as anything says.
type denial struct {
	verb        string
	resource    string // as can-i takes it: "pods", "deployments.apps"
	subresource string
	namespace   string
	// all is set for a namespaced resource asked across every namespace;
	// cluster for a request at cluster scope whose resource may be either.
	// Neither is set for a known cluster-scoped resource (isClusterScoped),
	// which has no namespaces to span.
	all     bool
	cluster bool
	path    string
}

func classifyForbidden(err error) Diagnosis {
	msg := apiMessage(err)
	d, ok := fromCall(err)
	if !ok {
		d, ok = fromMessage(msg)
	}
	if !ok {
		d = fromDetails(err)
	}
	return d.diagnosis(msg)
}

// fromCall takes the request from the k4s call that failed: it names the
// resource the way the user types it and knows whether it is namespaced.
func fromCall(err error) (denial, bool) {
	var call *CallError
	if !errors.As(err, &call) {
		return denial{}, false
	}
	return denial{
		verb:      sanitize.Line(call.Verb),
		resource:  sanitize.Line(call.Resource),
		namespace: sanitize.Line(call.Namespace),
		all:       call.Namespace == "" && !call.ClusterScoped && !isClusterScoped(call.Resource),
	}, true
}

func fromMessage(msg string) (denial, bool) {
	if m := deniedResource.FindStringSubmatch(msg); m != nil {
		resource, sub, _ := strings.Cut(m[2], "/")
		if m[3] != "" {
			resource += "." + m[3]
		}
		return denial{
			verb:        sanitize.Line(m[1]),
			resource:    sanitize.Line(resource),
			subresource: sanitize.Line(sub),
			namespace:   sanitize.Line(m[4]),
			cluster:     m[5] != "" && !isClusterScoped(resource),
		}, true
	}
	if m := deniedPath.FindStringSubmatch(msg); m != nil {
		return denial{verb: sanitize.Line(m[1]), path: sanitize.Line(m[2])}, true
	}
	return denial{}, false
}

// fromDetails is the last resort: the resource from StatusDetails, no verb.
func fromDetails(err error) denial {
	var status apierrors.APIStatus
	if !errors.As(err, &status) || status.Status().Details == nil {
		return denial{}
	}
	details := status.Status().Details
	resource := details.Kind
	if resource != "" && details.Group != "" {
		resource += "." + details.Group
	}
	return denial{resource: sanitize.Line(resource)}
}

func (d denial) diagnosis(msg string) Diagnosis {
	out := Diagnosis{
		Kind:          KindForbidden,
		Short:         "forbidden: " + d.what(msg),
		Verb:          d.verb,
		Resource:      d.resource,
		Namespace:     d.namespace,
		AllNamespaces: d.all,
	}
	out.Hint = d.canI().String()
	return out
}

// what names the refused request for Short.
func (d denial) what(msg string) string {
	switch {
	case d.path != "":
		return d.verb + " " + d.path
	case d.resource == "":
		if w := strings.TrimPrefix(redact(msg), "forbidden: "); w != "" {
			return w
		}
		return "request denied"
	case d.verb == "":
		return d.resource
	}
	what := d.verb + " " + d.resource
	if d.subresource != "" {
		what += "/" + d.subresource
	}
	switch {
	case d.namespace != "":
		return what + " in " + d.namespace
	case d.all:
		return what + " in all namespaces"
	case d.cluster:
		return what + " at cluster scope"
	}
	return what
}

// canI is the `kubectl auth can-i` that asks exactly what was refused, or
// lists what the user may do when the refusal does not say or says it in
// words can-i would not take as written.
func (d denial) canI() kubectl.Cmd {
	switch {
	case !d.askable():
		return kubectl.New("auth", "can-i", "--list")
	case d.path != "":
		return kubectl.New("auth", "can-i", d.verb, d.path)
	case d.verb == "" || d.resource == "":
		return kubectl.New("auth", "can-i", "--list")
	}
	args := []string{"auth", "can-i", d.verb, d.resource}
	if d.subresource != "" {
		args = append(args, "--subresource="+d.subresource)
	}
	switch {
	case d.namespace != "":
		args = append(args, "-n", d.namespace)
	case d.all, d.cluster:
		// -A is right for a namespaced resource and harmless for a
		// cluster-scoped one, so it serves when the call is not known.
		args = append(args, "-A")
	}
	return kubectl.New(args...)
}

// askable reports whether can-i takes every word of d as written. The
// apiserver writes the refusal, so a verb, resource or namespace such as
// --server=… would read as a flag in the hint the user types.
func (d denial) askable() bool {
	return canIWord(d.verb) && canIWord(d.resource) &&
		(d.subresource == "" || kubectl.CheckLabel("subresource", d.subresource) == nil) &&
		(d.namespace == "" || kubectl.CheckLabel("namespace", d.namespace) == nil) &&
		(d.path == "" || strings.HasPrefix(d.path, "/"))
}

// canIWord reports a verb or resource can-i takes: "" (not known), "*",
// or a DNS-1123 subdomain such as "list" or "deployments.apps".
func canIWord(s string) bool {
	return s == "" || s == "*" || kubectl.CheckSubdomain("word", s) == nil
}
