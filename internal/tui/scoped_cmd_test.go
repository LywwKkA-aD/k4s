package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestScopedCommand: `:<list> <ns>` opens the list in that namespace and
// `:<list> -A` in every namespace, from any view, as kubectl spells them.
func TestScopedCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		on      string // command run first, "" stays on the dashboard
		input   string
		view    string
		ns      string
		command string
	}{
		{name: "pods in a namespace", input: "pods demo", view: "pods", ns: "demo", command: "kubectl get pods -n demo"},
		{name: "an alias and -n", input: "deploy -n demo", view: "deployments", ns: "demo", command: "kubectl get deployments -n demo"},
		{name: "services", input: "svc kube-system", view: "services", ns: "kube-system", command: "kubectl get services -n kube-system"},
		{name: "top", input: "top demo", view: "top", ns: "demo", command: "kubectl top pods -n demo --sort-by=cpu"},
		{name: "events", input: "ev demo", view: "events", ns: "demo", command: "kubectl events -n demo"},
		{name: "every namespace", on: "ns demo", input: "pods -A", view: "pods", ns: "ALL", command: "kubectl get pods -A"},
		{name: "every namespace, long flag", on: "ns demo", input: "deploy --all-namespaces", view: "deployments", ns: "ALL", command: "kubectl get deployments -A"},
		{name: "a namespace called all", input: "svc all", view: "services", ns: "all", command: "kubectl get services -n all"},
		{name: "another list from a list", on: "pods", input: "deploy demo", view: "deployments", ns: "demo", command: "kubectl get deployments -n demo"},
		{name: "the same list in another namespace", on: "pods", input: "pods demo", view: "pods", ns: "demo", command: "kubectl get pods -n demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := started(t, tuitest.NewFakeClient(t))
			if tc.on != "" {
				m, _ = tuitest.Drive(t, m, colon(tc.on)...)
			}
			m, _ = tuitest.Drive(t, m, colon(tc.input)...)
			requireView(t, m, tc.view)
			if got := tuitest.HeaderNamespace(m); got != tc.ns {
				t.Errorf("header namespace = %q, want %q", got, tc.ns)
			}
			if got := commandRow(m); got != tc.command {
				t.Errorf("command row = %q, want %q", got, tc.command)
			}
		})
	}
}

// TestScopedCommandGoesBackWithEsc: the view and namespace before the
// command are one Esc away, and a later list without a namespace stays in
// the one asked for.
func TestScopedCommandGoesBackWithEsc(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("svc"), colon("deploy demo"))...)
	requireView(t, m, "deployments")

	next, _ := tuitest.Drive(t, m, colon("pods")...)
	if got := commandRow(next); got != "kubectl get pods -n demo" {
		t.Errorf(":pods after :deploy demo = %q, want it in demo", got)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "services")
	if got := commandRow(m); got != "kubectl get services -A" {
		t.Errorf("command row after Esc = %q, want services in every namespace", got)
	}
}

// TestScopedCommandToWhereTheListIs: asking for the list already shown
// changes nothing and says so, as :ns does.
func TestScopedCommandToWhereTheListIs(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods demo"), colon("pods demo"))...)
	if row := statusRow(footer(m)); row != "already in demo" {
		t.Errorf("status row = %q, want already in demo", row)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if got := tuitest.HeaderView(m); got != "dashboard" {
		t.Errorf("Esc went to %q, want the dashboard: the repeat left no entry", got)
	}
	if !strings.Contains(header(m), "ns ALL") {
		t.Errorf("header = %q, want every namespace back", header(m))
	}
}
