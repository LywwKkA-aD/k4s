package tui_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Bracketed paste reaches whichever text field has the keyboard, and only
// that: pasted text never runs a key binding.
func TestPasteGoesWhereTypingGoes(t *testing.T) {
	t.Parallel()

	logs := views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}, Tail: 10}
	cases := []struct {
		name   string
		input  []tea.Msg
		view   string
		want   []string
		refute []string
	}{
		{
			name:  "command bar",
			input: []tea.Msg{tuitest.Press(":"), tuitest.Paste("pods"), tuitest.Enter()},
			view:  "pods",
		},
		{
			name:   "list filter",
			input:  then(colon("pods"), []tea.Msg{tuitest.Press("/"), tuitest.Paste("web")}),
			view:   "pods",
			want:   []string{"/ web", tuitest.PodWeb1, tuitest.PodWeb2},
			refute: []string{tuitest.PodMulti},
		},
		{
			name:  "pasted text runs no binding",
			input: then(colon("pods"), []tea.Msg{tuitest.Paste("q")}),
			view:  "pods",
		},
		{
			name:  "help closes on any input",
			input: []tea.Msg{tuitest.Press("?"), tuitest.Paste("x")},
			view:  "dashboard",
			want:  []string{"q quit"},
		},
		{
			name:  "logs search",
			input: []tea.Msg{logs, tuitest.Press("/"), tuitest.Paste("needle"), tuitest.Enter()},
			view:  "pod · " + tuitest.PodWeb1 + " · logs · /needle · no match",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.input...)
			requireView(t, m, tc.view)
			requireScreen(t, m, tc.want...)
			refuteScreen(t, m, tc.refute...)
		})
	}
}
