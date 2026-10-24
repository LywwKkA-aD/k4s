package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// Names with bidi, zero-width and tag characters as well as escape
// sequences render inert in every panel the busy fixture fills, focused
// or not.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	feed := func() dash.Snapshot {
		s := busyK3s()
		s.Cluster.You.Text = tuitest.Hostile("devops")
		s.Nodes.Rows[0].Key.Name = tuitest.Hostile("k3s-cp-1")
		s.Pods.Problems[0].Key.Name = tuitest.Hostile("payments-worker")
		s.Pods.Problems[0].Status = tuitest.Hostile("CrashLoop")
		s.Workloads.Rows[0].Key.Name = tuitest.Hostile("payments-api")
		s.Events.Rows[0].Reason = tuitest.Hostile("BackOff")
		s.Events.Rows[0].Object.Name = tuitest.Hostile("loki-backend-0")
		return s
	}
	cases := []struct {
		name string
		keys []string
		want []string
	}{
		{name: "overview", want: []string{"devops", "k3s-cp-1", "payments-worker", "CrashLoop"}},
		{name: "pods focused", keys: []string{"3", "down"}, want: []string{"payments-worker", "CrashLoop"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(feed), wideW, wideH), tc.keys...)
			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q", w)
				}
			}
		})
	}
}
