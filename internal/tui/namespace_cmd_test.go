package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestNamespaceCommandToTheSameNamespace: asking for the namespace a list
// already shows changes nothing, leaves no history entry and says so.
func TestNamespaceCommandToTheSameNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup []tea.Msg
		input string
		want  string
		back  string // command row after one Esc
	}{
		{name: "named", setup: then(colon("deploy"), colon("ns demo")), input: "ns demo", want: "already in demo", back: "kubectl get deployments -A"},
		{name: "all namespaces", setup: colon("svc"), input: "ns -A", want: "already in all namespaces", back: "≈ kubectl get nodes,pods,deploy,sts,ds -A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.setup...)
			before := commandRow(m)
			m, out := tuitest.Drive(t, m, colon(tc.input)...)
			if row := statusRow(footer(m)); row != tc.want {
				t.Errorf("status row = %q, want %q", row, tc.want)
			}
			if got := commandRow(m); got != before {
				t.Errorf("command row = %q, want it unchanged at %q", got, before)
			}
			if reqs := tuitest.Requests(out); len(reqs) != 0 {
				t.Errorf("rebuilt the view:\n%s", tuitest.Describe(reqs))
			}
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			if got := commandRow(m); got != tc.back {
				t.Errorf("command row after Esc = %q, want %q", got, tc.back)
			}
		})
	}
}

// TestNamespaceCommandToTheSameNamespaceFromTheDashboard: only a
// namespaced list is left alone; elsewhere the command still opens pods.
func TestNamespaceCommandToTheSameNamespaceFromTheDashboard(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon("ns -A")...)
	requireView(t, m, "pods")
}

// TestNamespaceCommand: `:ns <name>` scopes the list view in place, or
// opens pods in that namespace from any other view; `:ns -A` is all
// namespaces, since "all" is a valid namespace name.
func TestNamespaceCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		on      string // command run first, "" stays on the dashboard
		input   string
		view    string
		ns      string
		command string
	}{
		{name: "from the dashboard", input: "ns demo", view: "pods", ns: "demo", command: "kubectl get pods -n demo"},
		{name: "deployments stay deployments", on: "deploy", input: "ns demo", view: "deployments", ns: "demo", command: "kubectl get deployments -n demo"},
		{name: "top stays top", on: "top", input: "namespaces demo", view: "top", ns: "demo", command: "kubectl top pods -n demo --sort-by=cpu"},
		{name: "all namespaces", on: "svc", input: "ns -A", view: "services", ns: "ALL", command: "kubectl get services -A"},
		{name: "a namespace called all", on: "pods", input: "ns all", view: "pods", ns: "all", command: "kubectl get pods -n all"},
		{name: "extra blanks and case of the command", input: "  NS   demo ", view: "pods", ns: "demo", command: "kubectl get pods -n demo"},
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
			if !strings.Contains(header(m), "ns "+tc.ns) {
				t.Errorf("header = %q", header(m))
			}
			if got := commandRow(m); got != tc.command {
				t.Errorf("command row = %q, want %q", got, tc.command)
			}
		})
	}
}

// TestNamespaceCommandGoesBackWithEsc: the view in the old namespace is
// one Esc away.
func TestNamespaceCommandGoesBackWithEsc(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("deploy"), colon("ns demo"))...)
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "deployments")
	if got := commandRow(m); got != "kubectl get deployments -A" {
		t.Errorf("command row after Esc = %q", got)
	}
}

func TestNamespaceCommandErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "not a namespace name", input: "ns Demo_1", want: `invalid namespace "Demo_1"`},
		{name: "two arguments", input: "ns demo shop", want: `namespaces takes one namespace, not "demo shop"`},
		{name: "a list with a bad namespace", input: "pods Demo_1", want: `invalid namespace "Demo_1"`},
		{name: "an unknown flag", input: "deploy --all", want: `deployments: unknown flag "--all"`},
		{name: "a view without namespaces", input: "dashboard demo", want: "dashboard takes no namespace"},
		{name: "nodes are cluster-scoped", input: "nodes demo", want: "nodes takes no namespace"},
		{name: "every namespace for nodes", input: "nodes -A", want: "nodes takes no namespace"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("svc"), colon(tc.input))...)
			requireView(t, m, "services")
			if got := footer(m); !strings.Contains(got, tc.want) {
				t.Errorf("footer = %q, want %q", got, tc.want)
			}
			if !strings.Contains(header(m), "ns ALL") {
				t.Errorf("header = %q", header(m))
			}
		})
	}
}
