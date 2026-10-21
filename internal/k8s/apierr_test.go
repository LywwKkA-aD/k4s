package k8s_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

const denied = `User "system:serviceaccount:k4s-demo:k4s-viewer" cannot `

func forbidden(resource, group, reason string) error {
	return apierrors.NewForbidden(schema.GroupResource{Group: group, Resource: resource}, "", errors.New(denied+reason))
}

func listCall(resource, ns string, err error) error {
	return &k8s.CallError{Verb: "list", Resource: resource, Namespace: ns, Err: err}
}

func dial(rawURL string, inner error) error {
	return &url.Error{Op: "Get", URL: rawURL, Err: &net.OpError{Op: "dial", Net: "tcp", Err: inner}}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		err   error
		kind  k8s.ErrorKind
		short string
		hint  string
	}{
		{name: "no error", err: nil, kind: k8s.KindNone},
		{
			name:  "forbidden in a namespace",
			err:   listCall("pods", "demo", forbidden("pods", "", `list resource "pods" in API group "" in the namespace "demo"`)),
			kind:  k8s.KindForbidden,
			short: "forbidden: list pods in demo",
			hint:  "kubectl auth can-i list pods -n demo",
		},
		{
			name:  "forbidden across all namespaces",
			err:   listCall("pods", "", forbidden("pods", "", `list resource "pods" in API group "" at the cluster scope`)),
			kind:  k8s.KindForbidden,
			short: "forbidden: list pods in all namespaces",
			hint:  "kubectl auth can-i list pods -A",
		},
		{
			name: "forbidden cluster-scoped resource",
			err: &k8s.CallError{Verb: "list", Resource: "namespaces", ClusterScoped: true,
				Err: forbidden("namespaces", "", `list resource "namespaces" in API group "" at the cluster scope`)},
			kind:  k8s.KindForbidden,
			short: "forbidden: list namespaces",
			hint:  "kubectl auth can-i list namespaces",
		},
		{
			name:  "forbidden named by the server alone",
			err:   forbidden("deployments", "apps", `list resource "deployments" in API group "apps" in the namespace "demo"`),
			kind:  k8s.KindForbidden,
			short: "forbidden: list deployments.apps in demo",
			hint:  "kubectl auth can-i list deployments.apps -n demo",
		},
		{
			name:  "forbidden at cluster scope without the call",
			err:   forbidden("nodes", "", `list resource "nodes" in API group "" at the cluster scope`),
			kind:  k8s.KindForbidden,
			short: "forbidden: list nodes at cluster scope",
			hint:  "kubectl auth can-i list nodes -A",
		},
		{
			name:  "forbidden subresource",
			err:   forbidden("pods", "", `get resource "pods/log" in API group "" in the namespace "demo"`),
			kind:  k8s.KindForbidden,
			short: "forbidden: get pods/log in demo",
			hint:  "kubectl auth can-i get pods --subresource=log -n demo",
		},
		{
			name:  "forbidden path",
			err:   apierrors.NewForbidden(schema.GroupResource{}, "", errors.New(denied+`get path "/version"`)),
			kind:  k8s.KindForbidden,
			short: "forbidden: get /version",
			hint:  "kubectl auth can-i get /version",
		},
		{
			name: "forbidden without a message, named by the call",
			err: listCall("pods.metrics.k8s.io", "",
				apierrors.NewGenericServerResponse(403, "get", schema.GroupResource{}, "", "unknown", 0, false)),
			kind:  k8s.KindForbidden,
			short: "forbidden: list pods.metrics.k8s.io in all namespaces",
			hint:  "kubectl auth can-i list pods.metrics.k8s.io -A",
		},
		{
			name:  "forbidden with nothing to go on",
			err:   apierrors.NewForbidden(schema.GroupResource{}, "", errors.New("no")),
			kind:  k8s.KindForbidden,
			short: "forbidden: no",
			hint:  "kubectl auth can-i --list",
		},
		{
			name:  "forbidden with an empty status",
			err:   &apierrors.StatusError{ErrStatus: metav1.Status{Status: metav1.StatusFailure, Code: 403, Reason: metav1.StatusReasonForbidden}},
			kind:  k8s.KindForbidden,
			short: "forbidden: request denied",
			hint:  "kubectl auth can-i --list",
		},
		{
			name:  "forbidden resource without a verb",
			err:   apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "db", errors.New("no")),
			kind:  k8s.KindForbidden,
			short: "forbidden: secrets",
			hint:  "kubectl auth can-i --list",
		},
		{
			name:  "forbidden grouped resource without a verb",
			err:   apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "web", errors.New("no")),
			kind:  k8s.KindForbidden,
			short: "forbidden: deployments.apps",
			hint:  "kubectl auth can-i --list",
		},
		{
			name:  "unauthorized",
			err:   listCall("pods", "demo", apierrors.NewUnauthorized("Unauthorized")),
			kind:  k8s.KindUnauthorized,
			short: "unauthorized: credentials expired or revoked",
			hint:  "log in again, then press r",
		},
		{
			name:  "credential plugin failed",
			err:   fmt.Errorf(`Get "https://a:6443/api": getting credentials: exec: executable /opt/bin/aws failed with exit code 255`),
			kind:  k8s.KindUnauthorized,
			short: "credentials: plugin aws failed (exit 255)",
			hint:  "log in with kubectl in a terminal, then press r",
		},
		{
			name:  "credential plugin missing",
			err:   fmt.Errorf("getting credentials: exec: executable kubelogin not found\n\nIt looks like you are trying to use..."),
			kind:  k8s.KindUnauthorized,
			short: "credentials: plugin kubelogin not found",
			hint:  "install it or fix users[].user.exec.command in the kubeconfig",
		},
		{
			name:  "credential plugin answered nonsense",
			err:   fmt.Errorf("getting credentials: exec plugin didn't return a status field"),
			kind:  k8s.KindUnauthorized,
			short: "credentials: exec plugin didn't return a status field",
			hint:  "log in with kubectl in a terminal, then press r",
		},
		{
			name:  "plugin needs a terminal, with k4s's hint",
			err:   fmt.Errorf("getting credentials: exec plugin cannot support interactive mode: standard input is unavailable: log in first: run `kubectl --context eks get ns` in a terminal, then press r"),
			kind:  k8s.KindLogin,
			short: "login required: the credential plugin needs a terminal",
			hint:  "log in first: run `kubectl --context eks get ns` in a terminal, then press r",
		},
		{
			name:  "plugin needs a terminal, bare",
			err:   fmt.Errorf("exec plugin cannot support interactive mode: standard input is not a terminal"),
			kind:  k8s.KindLogin,
			short: "login required: the credential plugin needs a terminal",
			hint:  "log in with kubectl in a terminal, then press r",
		},
		{
			name:  "not found",
			err:   apierrors.NewNotFound(schema.GroupResource{Resource: "pods"}, "web-1"),
			kind:  k8s.KindNotFound,
			short: `pods "web-1" not found`,
		},
		{
			name:  "not found, raw",
			err:   apierrors.NewGenericServerResponse(404, "get", schema.GroupResource{}, "", "the server could not find the requested resource", 0, false),
			kind:  k8s.KindNotFound,
			short: "not found: the server could not find the requested resource",
		},
		{
			name:  "deadline",
			err:   listCall("pods", "", &url.Error{Op: "Get", URL: "https://a:6443/api/v1/pods", Err: context.DeadlineExceeded}),
			kind:  k8s.KindTimeout,
			short: "API slow: list pods timed out",
			hint:  "r retries",
		},
		{
			name:  "server timeout",
			err:   apierrors.NewServerTimeout(schema.GroupResource{Resource: "pods"}, "list", 1),
			kind:  k8s.KindTimeout,
			short: "API slow: request timed out",
			hint:  "r retries",
		},
		{
			name:  "gateway timeout",
			err:   apierrors.NewTimeoutError("slow", 1),
			kind:  k8s.KindTimeout,
			short: "API slow: request timed out",
			hint:  "r retries",
		},
		{
			name:  "network timeout",
			err:   &url.Error{Op: "Get", URL: "https://a:6443/", Err: timeoutErr{}},
			kind:  k8s.KindTimeout,
			short: "API slow: request timed out",
			hint:  "r retries",
		},
		{
			name:  "refused",
			err:   listCall("pods", "", dial("https://10.0.0.5:6443/api/v1/pods?limit=500", syscall.ECONNREFUSED)),
			kind:  k8s.KindUnreachable,
			short: "API unreachable: 10.0.0.5:6443",
		},
		{
			name:  "no route",
			err:   dial("https://10.9.9.9:6443/version", syscall.EHOSTUNREACH),
			kind:  k8s.KindUnreachable,
			short: "API unreachable: 10.9.9.9:6443",
		},
		{
			name:  "dns",
			err:   &url.Error{Op: "Get", URL: "https://k8s.example.com/version", Err: &net.DNSError{Name: "k8s.example.com", Err: "no such host", IsNotFound: true}},
			kind:  k8s.KindUnreachable,
			short: "API unreachable: k8s.example.com",
		},
		{
			name:  "dial error without a URL",
			err:   &net.OpError{Op: "dial", Net: "tcp", Addr: &net.TCPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 6443}, Err: syscall.ECONNREFUSED},
			kind:  k8s.KindUnreachable,
			short: "API unreachable: 10.1.2.3:6443",
		},
		{
			name:  "dial error with an empty URL",
			err:   &url.Error{Op: "Get", URL: "", Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
			kind:  k8s.KindUnreachable,
			short: "API unreachable",
		},
		{
			name:  "dns error alone",
			err:   &net.DNSError{Name: "api.example.com", Err: "no such host", IsNotFound: true},
			kind:  k8s.KindUnreachable,
			short: "API unreachable: api.example.com",
		},
		{
			name:  "refused without any address",
			err:   fmt.Errorf("connect: %w", syscall.ECONNREFUSED),
			kind:  k8s.KindUnreachable,
			short: "API unreachable",
		},
		{
			name:  "k3s.yaml copied off the server",
			err:   dial("https://127.0.0.1:6443/version", syscall.ECONNREFUSED),
			kind:  k8s.KindUnreachable,
			short: "API unreachable: 127.0.0.1:6443",
			hint:  "127.0.0.1:6443 is the k3s server's own address: in a k3s.yaml copied to this machine, set server: to https://<server-ip>:6443",
		},
		{
			name:  "throttled",
			err:   apierrors.NewTooManyRequests("slow down", 3),
			kind:  k8s.KindThrottled,
			short: "API throttled: too many requests (429)",
			hint:  "k4s slows down; r retries",
		},
		{
			name:  "unavailable",
			err:   apierrors.NewServiceUnavailable("the server is currently unable to handle the request"),
			kind:  k8s.KindUnavailable,
			short: "API unavailable (503)",
			hint:  "r retries",
		},
		{
			name:  "anything else",
			err:   listCall("pods", "demo", errors.New("etcd down")),
			kind:  k8s.KindOther,
			short: "list pods: etcd down",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := k8s.ClassifyOnHost(tc.err, false)
			if got.Kind != tc.kind || got.Short != tc.short || got.Hint != tc.hint {
				t.Errorf("Classify(%v)\n got  %v %q %q\n want %v %q %q", tc.err, got.Kind, got.Short, got.Hint, tc.kind, tc.short, tc.hint)
			}
		})
	}
}

// TestClassifyForbiddenScope is the key the dashboard parks a 403 under.
func TestClassifyForbiddenScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want k8s.Diagnosis
	}{
		{
			name: "all namespaces",
			err:  listCall("pods", "", forbidden("pods", "", `list resource "pods" in API group "" at the cluster scope`)),
			want: k8s.Diagnosis{Verb: "list", Resource: "pods", AllNamespaces: true},
		},
		{
			name: "one namespace",
			err:  listCall("services", "demo", forbidden("services", "", `list resource "services" in API group "" in the namespace "demo"`)),
			want: k8s.Diagnosis{Verb: "list", Resource: "services", Namespace: "demo"},
		},
		{
			name: "server message only",
			err:  forbidden("deployments", "apps", `watch resource "deployments" in API group "apps" in the namespace "demo"`),
			want: k8s.Diagnosis{Verb: "watch", Resource: "deployments.apps", Namespace: "demo"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := k8s.Classify(tc.err)
			if got.Verb != tc.want.Verb || got.Resource != tc.want.Resource ||
				got.Namespace != tc.want.Namespace || got.AllNamespaces != tc.want.AllNamespaces {
				t.Errorf("Classify = %+v, want scope %+v", got, tc.want)
			}
		})
	}
}

func TestClassifyUnreachableHost(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		url       string
		k3sServer bool
		hint      string
	}{
		{name: "copied k3s.yaml", url: "https://127.0.0.1:6443/version",
			hint: "127.0.0.1:6443 is the k3s server's own address: in a k3s.yaml copied to this machine, set server: to https://<server-ip>:6443"},
		{name: "copied k3s.yaml, localhost", url: "https://localhost:6443/version",
			hint: "localhost:6443 is the k3s server's own address: in a k3s.yaml copied to this machine, set server: to https://<server-ip>:6443"},
		{name: "on the k3s server", url: "https://127.0.0.1:6443/version", k3sServer: true,
			hint: "is k3s running? sudo systemctl status k3s"},
		{name: "another loopback port", url: "https://127.0.0.1:8443/version"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := k8s.ClassifyOnHost(dial(tc.url, syscall.ECONNREFUSED), tc.k3sServer)
			if got.Kind != k8s.KindUnreachable || got.Hint != tc.hint {
				t.Errorf("Classify = %v %q, want unreachable with hint %q", got.Kind, got.Hint, tc.hint)
			}
		})
	}
}

// TestClassifyAsksThisMachine: Classify itself looks for a k3s server here,
// and either answer names what to do about 127.0.0.1:6443.
func TestClassifyAsksThisMachine(t *testing.T) {
	t.Parallel()

	got := k8s.Classify(dial("https://127.0.0.1:6443/version", syscall.ECONNREFUSED))
	want := []string{
		k8s.ClassifyOnHost(dial("https://127.0.0.1:6443/version", syscall.ECONNREFUSED), true).Hint,
		k8s.ClassifyOnHost(dial("https://127.0.0.1:6443/version", syscall.ECONNREFUSED), false).Hint,
	}
	if got.Hint != want[0] && got.Hint != want[1] {
		t.Errorf("Hint = %q, want one of %q", got.Hint, want)
	}
}

// TestClassifyRedacts: what reaches the status row is one inert line with
// no URL query or credentials.
func TestClassifyRedacts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "url query and userinfo",
			err:  errors.New(`Get "https://admin:s3cret@10.0.0.1:6443/api/v1/pods?token=abc&limit=1": EOF`),
			want: `Get "https://10.0.0.1:6443/api/v1/pods": EOF`,
		},
		{
			name: "first line only",
			err:  errors.New("list pods: broken\nsecond line"),
			want: "list pods: broken",
		},
		{
			name: "escape sequences",
			err:  errors.New("list pods: \x1b]52;c;ZXZpbA==\x07boom\x1b[31m!"),
			want: "list pods: boom!",
		},
		{
			name: "forbidden names from the server",
			err:  forbidden("pods", "", "list resource \"pods\" in API group \"\" in the namespace \"de\x1b[2Jmo\""),
			want: "forbidden: list pods in demo · kubectl auth can-i list pods -n demo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := k8s.Classify(tc.err).Text(); got != tc.want {
				t.Errorf("Text = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDiagnosisText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		d    k8s.Diagnosis
		want string
	}{
		{d: k8s.Diagnosis{}, want: ""},
		{d: k8s.Diagnosis{Short: "list pods: etcd down"}, want: "list pods: etcd down"},
		{d: k8s.Diagnosis{Short: "API slow: request timed out", Hint: "r retries"}, want: "API slow: request timed out · r retries"},
	}
	for _, tc := range cases {
		if got := tc.d.Text(); got != tc.want {
			t.Errorf("Text(%+v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestCallError(t *testing.T) {
	t.Parallel()

	inner := apierrors.NewUnauthorized("Unauthorized")
	err := listCall("pods", "demo", inner)
	if err.Error() != "list pods: Unauthorized" {
		t.Errorf("Error = %q", err.Error())
	}
	if !errors.Is(err, inner) || !apierrors.IsUnauthorized(err) {
		t.Errorf("the API error is not reachable through %v", err)
	}
}

func TestErrorKindString(t *testing.T) {
	t.Parallel()

	if got := k8s.KindForbidden.String(); got != "forbidden" {
		t.Errorf("String = %q", got)
	}
	if got := k8s.ErrorKind(99).String(); got != "unknown" {
		t.Errorf("String(99) = %q", got)
	}
}
