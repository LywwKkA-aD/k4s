package dashboard_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestDegradedStates is §10.5 on the view: forbidden, absent, not ready,
// stale and namespace-scoped sources each say so in a glyph and words,
// and a denied or unanswered source never reads as 0.
func TestDegradedStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		base      func() dash.Snapshot
		w, h      int
		keys      []string
		want, not []string
	}{
		{
			name: "namespace-scoped user, wide",
			base: nsScopedEKS, w: wideW, h: wideH,
			want: []string{
				"◆ ns shop (cluster-wide list forbidden)  ● 41 pods ok  ● 9/9 deploy ready  ○ no metrics",
				"▸ next 1/2  kubectl auth can-i --list -n shop",
				"— nodes: list forbidden for shop-dev", "kubectl auth can-i list nodes",
				"pods per node (ns shop) still shown in 3 pods",
				"○ no metrics-server: metrics.k8s.io is not in discovery",
				"install metrics-server: kubectl apply -f https://github.com/kubernetes-sigs/", "re-checked every 60s and on r",
				"— ns         forbidden · :ns <name> to switch",
				"● storage    4/4 pvc Bound (ns shop)",
				"● network    12 svc in ns shop · no LoadBalancer",
				"41 · ns shop",
			},
			not: []string{"0/0 Ready", "all clear"},
		},
		{
			name: "namespace-scoped user, 120 wide",
			base: nsScopedEKS, w: 120, h: 34,
			want: []string{
				"◆ ns shop (cluster-wide list forbidden)  ● 41 pods ok  ● 9/9 deploy ready  ○ no metrics",
				"— nodes: list forbidden for shop-dev",
			},
		},
		{
			name: "no metrics-server on k3s",
			base: noMetricsK3s, w: wideW, h: wideH,
			want: []string{
				"○ no metrics-server: metrics.k8s.io is not in discovery",
				"metrics-server is bundled with k3s; disabled with --disable=metrics-server?",
				"re-checked every 60s and on r",
			},
			not: []string{"cluster cpu", "≈5m"},
		},
		{
			name: "no context namespace to fall back to",
			base: func() dash.Snapshot {
				s := nsScopedEKS()
				s.Scope = dash.Scope{}
				s.Pods = dash.PodsPanel{Avail: denied("auth", "can-i", "list", "pods", "-A"), Cmd: kc("get", "pods", "-A")}
				return s
			},
			w: wideW, h: wideH,
			want: []string{
				"— cluster-wide lists forbidden · :ns <name>",
				"— pods forbidden · :ns <name> to switch", "kubectl auth can-i list pods -A",
			},
			not: []string{"0 pods", "no pods"},
		},
		{
			name: "pods never answered",
			base: func() dash.Snapshot {
				s := busyK3s()
				s.Pods = dash.PodsPanel{Avail: dash.Avail{State: dash.StateStale, Stale: 15 * time.Second, Unanswered: true}, Cmd: s.Pods.Cmd}
				return s
			},
			w: wideW, h: wideH,
			want: []string{"○ pods: no answer yet · stale <1m"},
			not:  []string{"no pods in any namespace", "nothing needs attention"},
		},
		{
			name: "a stale panel keeps its data and says since when",
			base: func() dash.Snapshot {
				s := busyK3s()
				s.Pods.Avail = stale(2 * time.Minute)
				s.Workloads.Deploy.Avail = stale(time.Minute)
				s.Events.Avail = stale(3 * time.Minute)
				return s
			},
			w: wideW, h: wideH,
			want: []string{"185 · 33 ns · stale 2m", "payments-worker-6c9f7d8b5-xk2lp", "61/65 ready · stale 1m", "type=Warning · 1h · stale 3m"},
		},
		{
			name: "metrics not ready",
			base: func() dash.Snapshot {
				s := noMetricsK3s()
				s.Nodes.Metrics = dash.Avail{State: dash.StateNotReady}
				return s
			},
			w: wideW, h: wideH,
			want: []string{"○ metrics not ready"},
		},
		{
			name: "metrics throttled",
			base: func() dash.Snapshot {
				s := noMetricsK3s()
				s.Nodes.Metrics = dash.Avail{State: dash.StateThrottled}
				return s
			},
			w: wideW, h: wideH,
			want: []string{"○ metrics throttled"},
		},
		{
			name: "a scoped user with pods starting and metrics denied",
			base: func() dash.Snapshot {
				s := nsScopedEKS()
				s.Pods.Problems = []dash.PodRow{problem("shop", "web-1", "ContainerCreating", 0, 1, 0, 0, minute, "ip-10-0-1-17", dash.SevWaiting)}
				s.Nodes.Metrics = denied("auth", "can-i", "list", "nodes.metrics.k8s.io")
				return s
			},
			w: wideW, h: wideH,
			want: []string{"● 40 pods ok  ◇ 1 starting  ● 9/9 deploy ready  — metrics forbidden"},
		},
		{
			name: "a short denied nodes panel keeps the probe",
			base: nsScopedEKS, w: 80, h: 14, keys: []string{"2"},
			want: []string{"— nodes: list forbidden for shop-dev", "kubectl auth can-i list nodes"},
			not:  []string{"re-checked"},
		},
		{
			name: "stale panels folded to summary lines say so",
			base: func() dash.Snapshot {
				s := busyK3s()
				s.Workloads.Deploy.Avail = stale(time.Minute)
				return s
			},
			w: 100, h: 18,
			want: []string{"4 workloads  ▲ 61/65 ready · ▲ 1 job failed · ○ 1 cronjob suspended · stale 1m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(tt.base), tt.w, tt.h), tt.keys...)
			screen := tuitest.Screen(m)
			requireSize(t, screen, tt.w, tt.h)
			requireAll(t, screen, tt.want, tt.not)
		})
	}
}

// TestStaleBanner: when the cluster stops answering, the banner says since
// when and when k4s tries again, over the last data; before any answer it
// says there has been none.
func TestStaleBanner(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		answered bool
		keys     []string
		want     string
	}{
		{name: "after an answer", answered: true, want: "○ stale since 14:02 · retry 5s"},
		{name: "before any answer", want: "○ no answer since 14:02 · retry 5s"},
		// With watch off nothing retries on its own: only r asks again.
		{name: "watch off", answered: true, keys: []string{"w"}, want: "○ stale since 14:02 · paused, r retries"},
		{name: "watch off after r", answered: true, keys: []string{"w", "r"}, want: "○ stale since 14:02 · paused, r retries"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			base := func() dash.Snapshot {
				s := busyK3s()
				s.Ledger[dash.Pods] = dash.Usage{Fetched: true, State: dash.StateStale, Answered: tt.answered}
				s.Pods.Avail = dash.Avail{State: dash.StateStale, Stale: time.Minute, Unanswered: !tt.answered}
				return s
			}
			m, _ := press(t, mount(t, newFeed(base), wideW, wideH), tt.keys...)
			screen := tuitest.Screen(m)
			requireAll(t, screen, []string{tt.want, "▲ 3 CrashLoopBackOff"}, []string{"retry 0s"})
		})
	}
}

// TestStalePanelIsDimmed: a stale panel's rows lose their state colours
// (the glyphs and words stay), so old data does not read as live.
func TestStalePanelIsDimmed(t *testing.T) {
	t.Parallel()

	failing := styles.Dashboard().Err.Render(styles.GlyphErr)
	rowOf := func(base func() dash.Snapshot) string {
		frame := tuitest.Frame(mount(t, newFeed(base), wideW, wideH))
		for _, l := range strings.Split(frame, "\n") {
			if strings.Contains(l, "payments-worker-6c9f7d8b5-r8vqn") {
				return l
			}
		}
		t.Fatal("no row for payments-worker-6c9f7d8b5-r8vqn")
		return ""
	}
	if !strings.Contains(rowOf(busyK3s), failing) {
		t.Skip("this terminal profile renders no colour")
	}
	staleRow := rowOf(func() dash.Snapshot {
		s := busyK3s()
		s.Pods.Avail = stale(2 * time.Minute)
		return s
	})
	if strings.Contains(staleRow, failing) {
		t.Fatalf("a stale row still paints ▲ as failing: %q", staleRow)
	}
	if !strings.Contains(tuitest.Strip(staleRow), "▲") {
		t.Fatalf("a stale row lost its glyph: %q", tuitest.Strip(staleRow))
	}
}
