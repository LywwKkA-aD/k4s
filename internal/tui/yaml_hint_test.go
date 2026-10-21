package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestYAMLKeyHint: o is the root's, so the root's hints name it next to y
// wherever the view has an object for it, and nowhere else.
func TestYAMLKeyHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []tea.Msg
		shown bool
	}{
		{name: "pods", input: colon("pods"), shown: true},
		{name: "deployments", input: colon("deploy"), shown: true},
		{name: "namespaces", input: colon("ns"), shown: true},
		{name: "describe", input: []tea.Msg{views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}}, shown: true},
		{name: "dashboard"},
		{name: "contexts", input: colon("ctx")},
		{name: "top", input: colon("top")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.input...)
			hints, _, _ := strings.Cut(footer(m), "\n")
			if got := strings.Contains(hints, "y copy  ·  o yaml"); got != tc.shown {
				t.Errorf("o yaml hinted = %v, want %v: %q", got, tc.shown, hints)
			}
		})
	}
}

// TestHelpListsTheYAMLKey: the help's global keys have o.
func TestHelpListsTheYAMLKey(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tuitest.Press("?"))
	requireScreen(t, m, "o       YAML of the selected object")
}
