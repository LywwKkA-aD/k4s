package stale_test

import (
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// listForbidden is RBAC refusing to list resource in ns ("" for all).
func listForbidden(resource, ns string) error {
	scope := "at the cluster scope"
	if ns != "" {
		scope = `in the namespace "` + ns + `"`
	}
	denial := errors.New(`User "u" cannot list resource "` + resource + `" in API group "" ` + scope)
	return &k8s.CallError{Verb: "list", Resource: resource, Namespace: ns,
		Err: apierrors.NewForbidden(schema.GroupResource{Resource: resource}, "", denial)}
}

func refused(host string) error {
	return &k8s.CallError{Verb: "list", Resource: "pods", Err: &url.Error{
		Op: "Get", URL: "https://" + host + "/api/v1/pods?limit=500",
		Err: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
	}}
}

func TestDescribe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "no error", err: nil, want: ""},
		{name: "plain", err: errDown, want: "list pods: connection refused"},
		{
			name: "forbidden in a namespace",
			err:  listForbidden("pods", "demo"),
			want: "forbidden: list pods in demo · kubectl auth can-i list pods -n demo",
		},
		{
			name: "forbidden across all namespaces points at :ns",
			err:  listForbidden("pods", ""),
			want: "forbidden: list pods in all namespaces · :ns <name> picks one · kubectl auth can-i list pods -A",
		},
		{name: "unreachable", err: refused("10.0.0.1:6443"), want: "API unreachable: 10.0.0.1:6443"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := stale.Describe(tc.err); got != tc.want {
				t.Errorf("Describe = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFoldSaysTheClassifiedText: the status and the error body read the
// same classified text.
func TestFoldSaysTheClassifiedText(t *testing.T) {
	t.Parallel()

	err := listForbidden("services", "demo")
	h, said := fold(t, err)
	want := "forbidden: list services in demo · kubectl auth can-i list services -n demo"
	if len(said[0]) != 1 || said[0][0] != (views.StatusMsg{Level: status.Error, Text: want}) {
		t.Errorf("Fold said %v, want %q", said[0], want)
	}
	if h.Text() != want {
		t.Errorf("Text = %q, want %q", h.Text(), want)
	}
	if h, _ = h.Fold(nil); h.Text() != "" {
		t.Errorf("Text after a success = %q", h.Text())
	}
}

func TestBannerIsClassified(t *testing.T) {
	t.Parallel()

	h, _ := fold(t, nil, refused("10.0.0.1:6443"))
	if got, want := tuitest.Strip(h.Banner()), "stale: API unreachable: 10.0.0.1:6443 · r retry"; got != want {
		t.Errorf("Banner = %q, want %q", got, want)
	}
}

func TestFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		err       error
		contextNS string
		want      string // the status; "" when there is no fallback
	}{
		{
			name: "forbidden across all namespaces, context names one",
			err:  listForbidden("pods", ""), contextNS: "demo",
			want: "cluster-wide list forbidden; showing demo · kubectl auth can-i list pods -A",
		},
		{name: "context names none", err: listForbidden("pods", "")},
		{name: "forbidden in a namespace", err: listForbidden("pods", "other"), contextNS: "demo"},
		{name: "another error", err: errDown, contextNS: "demo"},
		{name: "no error", contextNS: "demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			say, ok := stale.Fallback(tc.err, tc.contextNS)
			if ok != (tc.want != "") {
				t.Fatalf("Fallback ok = %v, want %v", ok, tc.want != "")
			}
			if !ok {
				return
			}
			got := tuitest.All[views.StatusMsg](tuitest.Collect(t, say))
			if len(got) != 1 || got[0] != (views.StatusMsg{Level: status.Warn, Text: tc.want}) {
				t.Errorf("Fallback said %v, want %q", got, tc.want)
			}
		})
	}
}
