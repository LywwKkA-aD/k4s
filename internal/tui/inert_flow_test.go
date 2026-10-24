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

// Among several pods, o and y act on the raw pod under the cursor after a
// filter: a clean row's YAML and copy name it, the hostile row's give no
// command at all, never one for a pod with the cleaned name.
func TestOAndYTargetTheRawPodUnderTheCursor(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	objs := tuitest.WithObjects(
		tuitest.Pod(demo, "api", nil, tuitest.ContainerNginx),
		tuitest.Pod(demo, "pay-db", nil, tuitest.ContainerNginx),
		tuitest.Pod(demo, tuitest.Hostile("pay-web"), nil, tuitest.ContainerNginx),
	)
	const dbYAML = "kubectl get pod pay-db -n demo -o yaml"
	cases := []struct {
		name     string
		cursor   []tea.Msg
		key      string
		wantCopy string // "" when nothing may be copied
	}{
		{name: "o on a clean row", cursor: tuitest.FilterBy("pay"), key: "o", wantCopy: dbYAML},
		{name: "o on the hostile row", cursor: then(tuitest.FilterBy("pay"), tuitest.Keys("down")), key: "o"},
		{name: "describe on the hostile row", cursor: then(tuitest.FilterBy("pay"), tuitest.Keys("down")), key: "enter"},
		{name: "o on the hostile row alone under its shown name", cursor: tuitest.FilterBy("web"), key: "o"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := started(t, tuitest.NewFakeClient(t, objs))
			m, _ = tuitest.Drive(t, m, then(colon("pods"), tc.cursor, tuitest.Keys(tc.key))...)
			tuitest.AssertInert(t, tuitest.Frame(m))
			m, msgs := tuitest.Drive(t, m, tuitest.Press("y"))
			got, copied := tuitest.Clipboard(msgs)
			switch {
			case tc.wantCopy == "" && copied:
				t.Errorf("y copied %q, want nothing for the hostile row", got)
			case tc.wantCopy != "" && got != tc.wantCopy:
				t.Errorf("y copied %q, %v, want %q", got, copied, tc.wantCopy)
			}
			if f := footer(m); tc.wantCopy == "" && strings.Contains(f, "pay-web") {
				t.Errorf("footer names the cleaned pod: %q", f)
			}
		})
	}
}
