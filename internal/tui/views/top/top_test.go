package top_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
)

const (
	width  = 120
	height = 30
)

// fresh builds the view on a manual clock, so no test waits on a tick.
func fresh(c *k8s.Client, namespace string) (top.Model, *asynctest.Clock) {
	clk := asynctest.NewClock(time.Time{})
	return top.New(c, namespace, top.WithClock(clk)), clk
}

// clocked is fresh, sized and started.
func clocked(t *testing.T, c *k8s.Client, namespace string) (top.Model, *asynctest.Clock) {
	t.Helper()
	m, clk := fresh(c, namespace)
	m, _ = tuitest.Mount(t, m, width, height)
	return m, clk
}

func mounted(t *testing.T, c *k8s.Client, namespace string) top.Model {
	t.Helper()
	m, _ := clocked(t, c, namespace)
	return m
}

func TestPodMetricsSortedByCPU(t *testing.T) {
	t.Parallel()

	screen := tuitest.Screen(mounted(t, tuitest.NewFakeClient(t), ""))
	for _, w := range []string{"NAMESPACE", "CPU", "MEMORY", tuitest.PodCoreDNS, "30m", "64Mi"} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	busiest := strings.Index(screen, tuitest.PodMulti)
	idle := strings.Index(screen, tuitest.PodCrash)
	if busiest < 0 || idle < 0 || busiest > idle {
		t.Errorf("rows are not sorted by CPU, busiest first:\n%s", screen)
	}
}

func TestNamespaceScope(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	screen := tuitest.Screen(m)
	if strings.Contains(screen, tuitest.PodCoreDNS) || strings.Contains(screen, "NAMESPACE") {
		t.Errorf("namespace-scoped top shows other namespaces:\n%s", screen)
	}
	if got := m.Command().String(); got != "kubectl top pods -n demo --sort-by=cpu" {
		t.Errorf("Command = %q", got)
	}
}

func TestToggleNodes(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), "")
	if got := m.Command().String(); got != "kubectl top pods -A --sort-by=cpu" {
		t.Errorf("pods mode = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
	if got := m.Command().String(); got != "kubectl top nodes --sort-by=cpu" {
		t.Errorf("after n = %q", got)
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, tuitest.NodeName) || !strings.Contains(screen, "250m") || strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("nodes mode screen:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
	if !strings.Contains(tuitest.Screen(m), tuitest.PodWeb1) {
		t.Errorf("second n returns to pods:\n%s", tuitest.Screen(m))
	}
}

func TestPlaceholders(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    top.Model
		want string
	}{
		{name: "no kubeconfig", m: top.New(nil, ""), want: "no kubeconfig"},
		{name: "loading", m: top.New(tuitest.NewFakeClient(t), ""), want: "loading metrics"},
		{name: "no samples yet", m: mounted(t, tuitest.NewFakeClient(t), "empty-ns"), want: "no metrics yet"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if screen := tuitest.Screen(tc.m); !strings.Contains(screen, tc.want) {
				t.Errorf("screen = %q, want %q", screen, tc.want)
			}
		})
	}
}

func TestRefreshRespectsBusy(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), "")
	m, first := tuitest.Step(t, m, tuitest.Press("r"))
	if len(first) != 1 {
		t.Fatalf("r should issue one fetch:\n%s", tuitest.Describe(first))
	}
	m, second := tuitest.Step(t, m, tuitest.Press("r"))
	if want := (views.StatusMsg{Level: status.Info, Text: "refresh already running"}); len(second) != 1 || second[0] != want {
		t.Fatalf("r while busy must only say so, not fetch again:\n%s", tuitest.Describe(second))
	}
	m, _ = tuitest.Drive(t, m, first...)
	if _, third := tuitest.Step(t, m, tuitest.Press("r")); len(third) != 1 {
		t.Errorf("the reply releases busy:\n%s", tuitest.Describe(third))
	}
}

func TestStaleRepliesAreDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	old, _ := fresh(c, "")
	_, oldReplies := tuitest.Init(t, old)
	newer, _ := fresh(c, "")
	newer, _ = tuitest.Drive(t, newer, tuitest.Resize(width, height))
	newer, _ = tuitest.Drive(t, newer, oldReplies...)
	if !strings.Contains(tuitest.Screen(newer), "loading metrics") {
		t.Errorf("a reply addressed to an older view was applied:\n%s", tuitest.Screen(newer))
	}

	// A pods reply that lands after the switch to nodes mode is dropped too.
	m, _ := fresh(c, "")
	podReply := tuitest.Collect(t, m.Init())
	m, _ = tuitest.Step(t, m, tuitest.Press("n"))
	m, _ = tuitest.Drive(t, m, podReply...)
	if screen := tuitest.Screen(m); strings.Contains(screen, tuitest.PodWeb1) {
		t.Errorf("pod metrics rendered in nodes mode:\n%s", screen)
	}
}

func TestWatchToggleAndContract(t *testing.T) {
	t.Parallel()

	m := top.New(nil, "")
	m, out := tuitest.Drive(t, m, tuitest.Press("w"), tuitest.Press("n"), tuitest.Press("s"), tuitest.Press("r"))
	if len(out) != 0 {
		t.Errorf("keys without a client emitted:\n%s", tuitest.Describe(out))
	}
	if m.Title() != "top" || m.CapturesKeys() || m.Close() != nil {
		t.Errorf("contract: title=%q", m.Title())
	}
	if got, want := tuitest.HelpKeys(m), []string{"n", "s", "w", "r"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("help keys = %v, want %v", got, want)
	}
}

func TestMetricsServerMissingHint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		status   int
		want     string
		wantHint bool
	}{
		{name: "metrics-server missing", status: http.StatusNotFound, want: "metrics unavailable: metrics-server is not installed", wantHint: true},
		{name: "metrics-server down", status: http.StatusServiceUnavailable, want: "metrics unavailable: metrics-server is not ready (503)"},
		{name: "forbidden", status: http.StatusForbidden, want: "kubectl auth can-i list pods.metrics.k8s.io -A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			screen := tuitest.Screen(mounted(t, tuitest.NewFakeClient(t, tuitest.WithMetricsStatus(tc.status)), ""))
			if !strings.Contains(strings.Join(strings.Fields(screen), " "), tc.want) {
				t.Errorf("screen lacks %q:\n%s", tc.want, screen)
			}
			if got := strings.Contains(screen, "install metrics-server"); got != tc.wantHint {
				t.Errorf("install hint shown = %v, want %v:\n%s", got, tc.wantHint, screen)
			}
		})
	}
}
