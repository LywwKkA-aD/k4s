package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestMain's kubeconfig holds the contexts alpha and beta, both pointing at
// a server that refuses connections.

func TestContextSwitchFromThePicker(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)),
		views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo})
	m, _ = tuitest.Drive(t, m, colon("ctx")...)
	requireView(t, m, "contexts")
	requireScreen(t, m, "alpha-cluster", "beta-cluster")

	m, out := tuitest.Drive(t, m, tuitest.Press("down"), tuitest.Enter())
	if !tuitest.Contains(out, views.ContextSelectedMsg{Name: "beta"}) {
		t.Fatalf("picker did not select beta:\n%s", tuitest.Describe(out))
	}
	requireView(t, m, "dashboard")
	// The new context's own namespace, as kubectl would use it.
	h := header(m)
	if !strings.Contains(h, "ctx: beta") || !strings.Contains(h, "ns: "+tuitest.NamespaceDemo) {
		t.Errorf("header after the switch = %q", h)
	}

	// The old cluster's history is gone.
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "dashboard")
	if !strings.Contains(header(m), "ctx: beta") {
		t.Errorf("esc after the switch changed context: %q", header(m))
	}
}

func TestContextSwitchWithoutAClient(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, nil), views.ContextSelectedMsg{Name: "alpha"})
	if !strings.Contains(header(m), "ctx: alpha") {
		t.Errorf("header = %q", header(m))
	}
	refuteScreen(t, m, "no kubeconfig")
}

func TestContextSwitchFailureKeepsTheCurrentClient(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon("pods")...)
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "nope"})
	if got := footer(m); !strings.Contains(got, `context: context "nope" not found in kubeconfig`) {
		t.Errorf("footer = %q", got)
	}
	requireView(t, m, "pods")
	if !strings.Contains(header(m), "ctx: "+tuitest.FakeContext) {
		t.Errorf("header = %q", header(m))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "dashboard")
}
