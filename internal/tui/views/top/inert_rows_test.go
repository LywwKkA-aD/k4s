package top_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostileHog is the raw name of the one hostile pod among clean ones.
var hostileHog = tuitest.Hostile("mem-hog")

func usage(name, cpu, mem string) k8stest.PodMetrics {
	return k8stest.PodMetrics{Namespace: "demo", Name: name, Containers: []k8stest.ContainerUsage{{Name: "app", CPU: cpu, Memory: mem}}}
}

// mixedUsage lists, by CPU, cpu-hog, the hostile mem-hog and idle; by
// memory, the hostile mem-hog, idle and cpu-hog.
func mixedUsage() []k8stest.PodMetrics {
	return []k8stest.PodMetrics{
		usage("cpu-hog", "300m", "10Mi"),
		usage(hostileHog, "200m", "900Mi"),
		usage("idle", "100m", "50Mi"),
	}
}

// The cursor keeps the raw key of its row while s re-sorts the rows, so
// the row it stands on is the same pod in either order.
func TestSortKeepsTheCursorOnTheRawPod(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		keys       []string
		wantKey    string
		wantCursor int
	}{
		{name: "by cpu, a clean row", keys: nil, wantKey: views.RowKey("demo", "cpu-hog"), wantCursor: 0},
		{name: "by cpu, the hostile row", keys: []string{"down"}, wantKey: views.RowKey("demo", hostileHog), wantCursor: 1},
		{name: "by memory, the hostile row moves first", keys: []string{"down", "s"}, wantKey: views.RowKey("demo", hostileHog), wantCursor: 0},
		{name: "by memory, a clean row moves last", keys: []string{"s", "down", "down"}, wantKey: views.RowKey("demo", "cpu-hog"), wantCursor: 2},
		{name: "by cpu again", keys: []string{"down", "s", "s"}, wantKey: views.RowKey("demo", hostileHog), wantCursor: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Metrics(mixedUsage(), nil))
			m, _ := tuitest.Drive(t, mounted(t, c, "demo"), tuitest.Keys(tc.keys...)...)
			tuitest.AssertInert(t, tuitest.Frame(m))
			if st := m.State(); st.Selected != tc.wantKey || st.Cursor != tc.wantCursor {
				t.Errorf("cursor on %q at %d, want %q at %d", st.Selected, st.Cursor, tc.wantKey, tc.wantCursor)
			}
		})
	}
}

// A poll whose usage puts other pods above the hostile one keeps the
// cursor on the same raw pod.
func TestPollKeepsTheCursorOnTheRawPod(t *testing.T) {
	t.Parallel()

	var srv changing
	srv.set(serving(mixedUsage()...))
	m, clk := clocked(t, k8stest.NewServerClient(t, &srv), "demo")
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	srv.set(serving(usage("cpu-hog", "300m", "10Mi"), usage("idle", "250m", "50Mi"), usage("new", "400m", "1Mi"), usage(hostileHog, "5m", "900Mi")))
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if st := m.State(); st.Selected != views.RowKey("demo", hostileHog) || st.Cursor != 3 {
		t.Errorf("after the poll the cursor is on %q at %d, want the hostile pod at 3", st.Selected, st.Cursor)
	}
}
