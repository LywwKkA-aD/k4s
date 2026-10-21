package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

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
		{name: "top stays top", on: "top", input: "namespaces demo", view: "top", ns: "demo", command: "kubectl top pods -n demo"},
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
			if !strings.Contains(header(m), "ns: "+tc.ns) {
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
		{name: "two arguments", input: "ns demo shop", want: `invalid namespace "demo shop"`},
		{name: "other commands take no argument", input: "pods demo", want: "unknown command: pods demo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("svc"), colon(tc.input))...)
			requireView(t, m, "services")
			if got := footer(m); !strings.Contains(got, tc.want) {
				t.Errorf("footer = %q, want %q", got, tc.want)
			}
			if !strings.Contains(header(m), "ns: ALL") {
				t.Errorf("header = %q", header(m))
			}
		})
	}
}
