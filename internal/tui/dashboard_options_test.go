package tui_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// TestDashboardOptionsReachEveryDashboard: the options of every
// WithDashboardOptions go to each dashboard the root builds: the first,
// whose bars spring up on the motion clock, and one built on going home,
// which sweeps the panel tab focuses; its refreshes run the fetch given.
func TestDashboardOptionsReachEveryDashboard(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []tea.Msg
		// afterMount counts only what happened after the mount.
		afterMount bool
	}{
		{name: "the first dashboard"},
		{name: "a dashboard built on going home", input: then(colon("pods"), tuitest.Keys("q", "tab")), afterMount: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := tuitest.NewFakeClient(t)
			src, err := dash.NewSource(c)
			if err != nil {
				t.Fatal(err)
			}
			real := dash.NewFetcher(src).Fetch
			var fetched atomic.Int32
			fetch := func(ctx context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot {
				fetched.Add(1)
				return real(ctx, p, prev)
			}
			clk := &asynctest.Clock{}
			root := tui.New(c, nil,
				tui.WithDashboardOptions(dashboard.WithFrames(clk)),
				tui.WithDashboardOptions(dashboard.WithFetch(fetch)))
			m, _ := tuitest.Mount(t, root, screenW, screenH)
			frames, fetches := 0, int32(0)
			if tc.afterMount {
				frames, fetches = len(clk.Delays()), fetched.Load()
			}
			m, _ = tuitest.Drive(t, m, tc.input...)
			requireView(t, m, "dashboard")
			if armed := clk.Delays()[frames:]; !slices.Contains(armed, viz.FrameInterval) {
				t.Errorf("no frame was armed on the clock, delays %v", armed)
			}
			if fetched.Load() == fetches {
				t.Error("no refresh ran the fetch given")
			}
		})
	}
}
