package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// A hostile pod name stays inert across the whole frame, header, body,
// footer and popups, from the list through describe and its help; the
// describe view keeps the raw name, so its footer shows no command
// rather than one for another pod.
func TestHostilePodNameIsInertEverywhere(t *testing.T) {
	t.Parallel()

	pod := tuitest.Pod(tuitest.NamespaceDemo, tuitest.Hostile("web-1"), nil, tuitest.ContainerNginx)
	cases := []struct {
		name  string
		input []tea.Msg
		want  []string
	}{
		{name: "the pods list", input: colon("pods"), want: []string{"web-1"}},
		{name: "describe", input: then(colon("pods"), tuitest.Keys("enter")), want: []string{"pod · web-1"}},
		{name: "describe help", input: then(colon("pods"), tuitest.Keys("enter", "?")), want: []string{"view: pod · web-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := started(t, tuitest.NewFakeClient(t, tuitest.WithObjects(pod)))
			m, _ = tuitest.Drive(t, m, tc.input...)
			tuitest.AssertInert(t, tuitest.Frame(m))
			requireScreen(t, m, tc.want...)
			if f := footer(m); strings.Contains(f, "describe pod") {
				t.Errorf("footer shows a command for the cleaned name: %q", f)
			}
		})
	}
}
