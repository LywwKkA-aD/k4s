package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestOOnTheDashboard: o on a dashboard row of an object opens its YAML;
// on a panel, which stands for a list, the root says there is no row.
func TestOOnTheDashboard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		keys   []tea.Msg
		view   string
		cmd    string
		status string
	}{
		{
			name: "a pod row", keys: tuitest.Keys("3", "down", "o"),
			view: "pod · crash-0 · yaml", cmd: "kubectl get pod crash-0 -n demo -o yaml",
		},
		{name: "the pods panel", keys: tuitest.Keys("3", "o"), view: "dashboard", status: "yaml: no row selected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t)
			m, _ := tuitest.Mount(t, tui.New(c, nil, instantFrames(), answeredWith(t, c, crashRow)), screenW, screenH)
			m, _ = tuitest.Drive(t, m, tc.keys...)
			if got := viewName(m); !strings.HasPrefix(got, tc.view) {
				t.Fatalf("view = %q, want %q\n%s", got, tc.view, tuitest.Screen(m))
			}
			if tc.status != "" {
				if got := footer(m); !strings.Contains(got, tc.status) {
					t.Errorf("footer = %q, want %q", got, tc.status)
				}
				return
			}
			requireScreen(t, m, "apiVersion:", "kind: Pod", "name: "+tuitest.PodCrash)
			if got := kubectlRow(m); got != tc.cmd {
				t.Errorf("footer command = %q, want %q", got, tc.cmd)
			}
		})
	}
}
