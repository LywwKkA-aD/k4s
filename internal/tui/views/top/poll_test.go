package top_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
)

func hint(t *testing.T, m top.Model, k string) string {
	t.Helper()
	for _, b := range m.Help() {
		if b.Help().Key == k {
			return b.Help().Desc
		}
	}
	t.Fatalf("no %s binding in Help", k)
	return ""
}

func TestTicksRefreshOnTheCadence(t *testing.T) {
	t.Parallel()

	m, clk := clocked(t, tuitest.NewFakeClient(t), "")
	if got := hint(t, m, "w"); got != "watch 5s" {
		t.Errorf("w hint = %q", got)
	}
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	_, out := tuitest.Drive(t, m, tick)
	if n := len(tuitest.All[async.Result[[]k8s.PodMetric]](out)); n != 1 {
		t.Errorf("a tick fetched %d times, want once:\n%s", n, tuitest.Describe(out))
	}
	if got := clk.Delays(); !slices.Equal(got, []time.Duration{5 * time.Second, 5 * time.Second}) {
		t.Errorf("tick delays = %v", got)
	}
}

func TestWatchOffArmsNothing(t *testing.T) {
	t.Parallel()

	m, clk := clocked(t, tuitest.NewFakeClient(t), "")
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := hint(t, m, "w"); got != "paused" {
		t.Errorf("w hint after w = %q", got)
	}
	tick, _ := clk.Fire()
	if _, out := tuitest.Drive(t, m, tick); len(out) != 0 {
		t.Errorf("a tick armed before w fetched:\n%s", tuitest.Describe(out))
	}
}

// TestToggleRetiresTheOtherModesTick: n moves the poller to a new Token,
// so the tick armed for pods does not fetch in nodes mode.
func TestToggleRetiresTheOtherModesTick(t *testing.T) {
	t.Parallel()

	m, clk := clocked(t, tuitest.NewFakeClient(t), "")
	m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
	podsTick, _ := clk.Fire()
	nodesTick, _ := clk.Fire()
	m, out := tuitest.Drive(t, m, podsTick)
	if len(out) != 0 {
		t.Errorf("the pods tick fetched in nodes mode:\n%s", tuitest.Describe(out))
	}
	_, out = tuitest.Drive(t, m, nodesTick)
	if n := len(tuitest.All[async.Result[[]k8s.NodeMetric]](out)); n != 1 {
		t.Errorf("the nodes tick fetched %d times, want once:\n%s", n, tuitest.Describe(out))
	}
}

func skewed(t *testing.T) *k8s.Client {
	t.Helper()
	return k8stest.NewServerClient(t, k8stest.Metrics(
		[]k8stest.PodMetrics{
			{Namespace: "demo", Name: "cpu-hog", Containers: []k8stest.ContainerUsage{{Name: "app", CPU: "900m", Memory: "10Mi"}}},
			{Namespace: "demo", Name: "mem-hog", Containers: []k8stest.ContainerUsage{{Name: "app", CPU: "5m", Memory: "900Mi"}}},
		},
		[]k8stest.NodeMetrics{
			{Name: "cpu-node", CPU: "3000m", Memory: "1Gi"},
			{Name: "mem-node", CPU: "100m", Memory: "7Gi"},
		},
	))
}

func TestSortToggle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		keys        []tea.Msg
		first, last string
		command     string
	}{
		{name: "pods by cpu", first: "cpu-hog", last: "mem-hog", command: "kubectl top pods -n demo"},
		{name: "pods by memory", keys: tuitest.Keys("s"), first: "mem-hog", last: "cpu-hog", command: "kubectl top pods -n demo --sort-by=memory"},
		{name: "back to cpu", keys: tuitest.Keys("s", "s"), first: "cpu-hog", last: "mem-hog", command: "kubectl top pods -n demo"},
		{name: "nodes by memory", keys: tuitest.Keys("s", "n"), first: "mem-node", last: "cpu-node", command: "kubectl top nodes --sort-by=memory"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, mounted(t, skewed(t), "demo"), tc.keys...)
			screen := tuitest.Screen(m)
			if first, last := strings.Index(screen, tc.first), strings.Index(screen, tc.last); first < 0 || last < 0 || first > last {
				t.Errorf("want %s above %s:\n%s", tc.first, tc.last, screen)
			}
			if got := m.Command().String(); got != tc.command {
				t.Errorf("Command = %q, want %q", got, tc.command)
			}
		})
	}
}

func TestSortHintNamesTheColumn(t *testing.T) {
	t.Parallel()

	m := top.New(nil, "")
	if got := hint(t, m, "s"); got != "sort cpu" {
		t.Errorf("s hint = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("s"))
	if got := hint(t, m, "s"); got != "sort memory" {
		t.Errorf("s hint after s = %q", got)
	}
}

// TestMetricsFailuresAnswerWithAStatus: a failed list says why in the
// status row, once per failure rather than on every tick, and again when
// the user asks with r.
func TestMetricsFailuresAnswerWithAStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		status int
		want   views.StatusMsg
	}{
		{
			name: "metrics-server missing", status: http.StatusNotFound,
			want: views.StatusMsg{Level: status.Warn, Text: "metrics unavailable: metrics-server is not installed"},
		},
		{
			name: "forbidden", status: http.StatusForbidden,
			want: views.StatusMsg{Level: status.Error, Text: "metrics forbidden: metrics request: unknown"},
		},
		{
			name: "metrics-server down", status: http.StatusServiceUnavailable,
			want: views.StatusMsg{Level: status.Warn, Text: "metrics unavailable: metrics request: the server is currently unable to handle the request"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, clk := fresh(tuitest.NewFakeClient(t, tuitest.WithMetricsStatus(tc.status)), "")
			m, out := tuitest.Mount(t, m, width, height)
			if got := tuitest.All[views.StatusMsg](out); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("first failure: got %+v, want %+v", got, tc.want)
			}
			tick, _ := clk.Fire()
			m, out = tuitest.Drive(t, m, tick)
			if got := tuitest.All[views.StatusMsg](out); len(got) != 0 {
				t.Errorf("the same failure on a tick reported again: %+v", got)
			}
			_, out = tuitest.Drive(t, m, tuitest.Press("r"))
			if got := tuitest.All[views.StatusMsg](out); len(got) != 1 || got[0] != tc.want {
				t.Errorf("r: got %+v, want %+v", got, tc.want)
			}
		})
	}
}
