package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// wideW is wide enough for every view's hints, the last included.
const wideW = 400

// hintsRow is the footer's first row: the key hints.
func hintsRow(m tui.Model) string {
	hints, _, _ := strings.Cut(footer(m), "\n")
	return hints
}

// TestYAMLKeyHint: o is the root's, so the root's hints name it, after
// the view's own, wherever the view has an object for it, and nowhere
// else.
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
			m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil, instantFrames()), wideW, screenH)
			m, _ = tuitest.Drive(t, m, tc.input...)
			hints := hintsRow(m)
			if got := strings.HasSuffix(hints, "  ·  o yaml"); got != tc.shown {
				t.Errorf("o yaml hinted last = %v, want %v: %q", got, tc.shown, hints)
			}
			if !tc.shown && strings.Contains(hints, "o yaml") {
				t.Errorf("o yaml hinted where there is nothing to open: %q", hints)
			}
		})
	}
}

// TestViewHintsComeBeforeTheYAMLHint: the footer is cut to the width, so
// a global hint ahead of the view's would push them, the watch state among
// them, off a narrow terminal. The view's first hint follows y copy.
func TestViewHintsComeBeforeTheYAMLHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		width int
		input []tea.Msg
		want  string
	}{
		{name: "pods at 80 columns", width: 80, input: colon("pods"), want: "y copy  ·  ent"},
		{name: "describe at 80 columns", width: 80, input: []tea.Msg{views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}}, want: "y copy  ·  "},
		{name: "pods in a namespace at 160 columns", width: screenW, input: []tea.Msg{views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo}}, want: "w watch 5s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil, instantFrames()), tc.width, screenH)
			m, _ = tuitest.Drive(t, m, tc.input...)
			hints := hintsRow(m)
			if !strings.Contains(hints, tc.want) {
				t.Errorf("hints lack %q: %q", tc.want, hints)
			}
			if strings.Contains(hints, "y copy  ·  o y") {
				t.Errorf("o yaml comes before the view's hints: %q", hints)
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
