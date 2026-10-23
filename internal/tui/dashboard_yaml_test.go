package tui_test

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// withCrashRow answers the dashboard's refreshes from the fake cluster,
// with panel 3 listing crash-0 as the pods Table, which the fake does not
// serve, would.
func withCrashRow(t *testing.T, c *k8s.Client) tui.Option {
	t.Helper()
	src, err := dash.NewSource(c)
	if err != nil {
		t.Fatal(err)
	}
	fetch := dash.NewFetcher(src).Fetch
	crash := dash.PodRow{
		Key:      dash.ObjectKey{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodCrash},
		Resolved: true, Status: "CrashLoopBackOff", Sev: dash.SevFailing,
		Cmd: dash.Cmd{Args: []string{"describe", "pod", tuitest.PodCrash, "-n", tuitest.NamespaceDemo}},
	}
	return tui.WithDashboardOptions(dashboard.WithFetch(func(ctx context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot {
		s := fetch(ctx, p, prev)
		s.Pods = dash.PodsPanel{
			Avail: dash.Avail{State: dash.StateOK}, Cmd: dash.Cmd{Args: []string{"get", "pods", "-A"}},
			Total: 1, NotOK: 1, Phases: dash.PhaseCounts{Failing: 1}, Problems: []dash.PodRow{crash},
		}
		return s
	}))
}

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
			m, _ := tuitest.Mount(t, tui.New(c, nil, instantFrames(), withCrashRow(t, c)), screenW, screenH)
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
