package dashboard_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// body is the dashboard's share of a terminal: 6 rows go to the header
// and the footer, and the root never hands a view less than one row.
func body(w, h int) (int, int) { return w, max(h-6, 1) }

// TestLayoutGoldens pins the busy fixture at the spec's terminal sizes,
// one per layout: medium at 160x46 and 120x40 (focus as in the 120x40
// mockup), the compact accordion at 100x30 and 80x24 (as in the 80x24
// mockup), the summary lines at 60x14 and the verdict alone at 40x6.
// Regenerate with
//
//	go test ./internal/tui/views/dashboard -run Golden -update
func TestLayoutGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		w, h int
		keys []string
	}{
		{w: 160, h: 46},
		{w: 120, h: 40, keys: []string{"2"}},
		{w: 100, h: 30},
		{w: 80, h: 24, keys: []string{"3", "down"}},
		{w: 60, h: 14},
		{w: 40, h: 6},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("busy_k3s_%dx%d", tt.w, tt.h)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			w, h := body(tt.w, tt.h)
			screen := warm(t, w, h, tt.keys...)
			requireSize(t, screen, w, h)
			tuitest.Golden(t, name, screen+"\n")
		})
	}
}

// TestMediumLayout is the 120x40 mockup: a two-row banner with chips
// grouped under one glyph per severity, narrow cluster labels whose
// values lose whole parts, nodes with their pods and the history as two
// spark rows, the pods list without the not-ok strip, and warnings on
// two lines each.
func TestMediumLayout(t *testing.T) {
	t.Parallel()

	screen := warm(t, 120, 34, "2")
	requireAll(t, screen, []string{
		"▲ attention  3 CrashLoopBackOff · 1 ImagePullBackOff · 4 workloads · 1 job  △ k3s-agent-1 mem 87% · +2",
		"▸ next 1/2  kubectl logs payments-worker-6c9f7d8b5-xk2lp -n shop -c worker --previous   why does it keep crashing?",
		"● api      ready · 8/8 checks · etcd ok",
		"◆ version  v1.36.3+k3s1 · embedded etcd ",
		"○ network  102 svc · 1 LoadBalancer <pending>",
		"PODS", "112/250",
		"cpu ≈30m ", "% of 36 cores", "mem ≈30m ", "% of 192 GiB",
		"≈ history from kubectl top nodes · △ ≥ 85%",
		"deploy 49/52 · sts 7/8 · ds 5/5 · jobs ▲1 · cron ○1",
		"◇ <1m  BackOff",
		"    Back-off restarting failed container ingester",
	}, []string{"≈30m not ok", "control pl", "LAST SEEN", "≈ last hour"})
}

// TestClusterValuesFit: where a cluster line's value does not fit, its
// note goes, then the short form stands in, then whole parts go, before
// anything is cut.
func TestClusterValuesFit(t *testing.T) {
	t.Parallel()

	base := func() dash.Snapshot {
		s := busyK3s()
		s.Cluster.Network.Avail = stale(2 * time.Minute)
		s.Cluster.Version.GitVersion = "v1.36.3+k3s1-rc.1-snapshot-20261021"
		return s
	}
	screen := tuitest.Screen(mount(t, newFeed(base), 100, 30))
	requireAll(t, screen, []string{
		"○ network  102 svc · 1 LB <pending> · stale 2m",
		"◆ version  v1.36.3+k3s1-rc.1-snapshot-20261021 ",
	}, []string{"embedded etc", "LoadBalancer"})
}

// TestCompactLayout is the 80x24 mockup: the banner keeps the chips and
// the next command, the accordion's open panel shows its rows under one
// bar line, and the rest are summary lines.
func TestCompactLayout(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, 80, 18)
	m = advance(t, m, f, 10*time.Minute)
	m, _ = press(t, m, "3", "down")
	screen := tuitest.Screen(m)
	lines := strings.Split(screen, "\n")
	if b := lines[0]; !strings.HasPrefix(b, " ▲ 3 CrashLoop · 1 ImagePull · 4 workloads · 1 job ") || strings.Contains(b, "watch") || strings.Contains(b, "attention") {
		t.Errorf("banner = %q, want the short chips without the word attention or the watch state", b)
	}
	// The command alone, cut at the edge; the footer has it whole.
	if want := " ▸ next  kubectl logs payments-worker-6c9f7d8b5-xk2lp -n shop -c worker --previ…"; lines[1] != want {
		t.Errorf("next line = %q, want %q", lines[1], want)
	}
	requireAll(t, screen, []string{
		"   1 cluster    ● api ready",
		"░▚·  █171 ▓7 ░2 ▚4 ·1",
		"›▲ shop/payments-worker-6c9f7d8b5-xk2lp  0/1    CrashLoopBackOff  41",
		"   4 workloads  ▲ 61/65 ready",
		"   5 events     △ 10 warnings in 1h · BackOff ×4",
	}, []string{"≈30m", "Running 171"})
	if got := hint(m, "w"); got != "5s" {
		t.Errorf("w hint = %q, want the short 5s", got)
	}
}

// TestAccordionOpensTheWorstPanel is the compact rule: on the first load
// the open panel is the first of pods, workloads, nodes and events with
// something failing, else pods; it never moves on its own after that.
func TestAccordionOpensTheWorstPanel(t *testing.T) {
	t.Parallel()

	calm := func(s *dash.Snapshot) {
		s.Pods.Problems = nil
		s.Workloads.Rows = nil
	}
	tests := []struct {
		name   string
		change func(*dash.Snapshot)
		want   string
	}{
		{name: "pods failing", change: func(*dash.Snapshot) {}, want: "3 pods"},
		{name: "only workloads failing", change: func(s *dash.Snapshot) { s.Pods.Problems = nil }, want: "4 workloads"},
		{name: "only a node failing", change: func(s *dash.Snapshot) {
			calm(s)
			s.Nodes.Rows[2].Condition, s.Nodes.Rows[2].Sev = "NotReady", dash.SevFailing
		}, want: "2 nodes"},
		{name: "only warnings", change: calm, want: "3 pods"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newFeed(func() dash.Snapshot {
				s := busyK3s()
				tt.change(&s)
				return s
			})
			m := mount(t, f, 80, 18)
			if got := openPanel(tuitest.Screen(m)); got != tt.want {
				t.Fatalf("open panel = %q, want %q", got, tt.want)
			}
			f.set(busyK3s)
			m = advance(t, m, f, 5*time.Second)
			if got := openPanel(tuitest.Screen(m)); got != tt.want {
				t.Fatalf("open panel moved to %q on a new snapshot, want %q", got, tt.want)
			}
		})
	}
}

// openPanel is the title of the one framed panel on a compact screen.
func openPanel(screen string) string {
	m := regexp.MustCompile(`[┤┫] (\d [a-z ]+?) [├┣]`).FindStringSubmatch(screen)
	if m == nil {
		return ""
	}
	return m[1]
}

// TestColumnDropOrder narrows the accordion's open pods, nodes and events
// panel one column at a time and checks that columns go in the spec's
// order: a column is never missing while one that drops before it is
// still shown.
func TestColumnDropOrder(t *testing.T) {
	t.Parallel()

	type step struct {
		first, then string // first drops before then: shown, it needs then
	}
	tests := []struct {
		panel, head string
		pairs       []step
	}{
		{panel: "3", head: "STATUS", pairs: []step{{"NODE", "AGE"}, {"AGE", " ago)"}, {" ago)", "NAMESPACE"}}},
		{panel: "5", head: "REASON", pairs: []step{{"MESSAGE", "COUNT"}, {"COUNT", " · monitoring"}}},
		{panel: "2", head: "STATUS", pairs: []step{{"ROLES", "≈5m"}, {"≈5m", "AGE"}, {"AGE", "PODS"}}},
	}
	for _, tt := range tests {
		t.Run("panel "+tt.panel, func(t *testing.T) {
			t.Parallel()
			m, _ := press(t, mount(t, newFeed(busyK3s), wideW, wideH), tt.panel)
			dropped := map[string]bool{}
			for w := 260; w >= 60; w-- {
				sized, _ := m.Update(tuitest.Resize(w, wideH))
				text := boxText(tuitest.Screen(sized), tt.head)
				for _, p := range tt.pairs {
					first, then := strings.Contains(text, p.first), strings.Contains(text, p.then)
					if first && !then {
						t.Fatalf("at %d wide %q is gone while %q, which drops before it, is shown:\n%s", w, p.then, p.first, text)
					}
					dropped[p.first] = dropped[p.first] || (!first && then)
				}
			}
			for _, p := range tt.pairs {
				if !dropped[p.first] {
					t.Errorf("no width shows %q without %q: the sweep tests nothing", p.then, p.first)
				}
			}
		})
	}
}

// boxText is the focused panel's table, cut out of the screen between its
// heavy side borders: the header line (the line with head) and the rows
// under it, without the history chart.
func boxText(screen, head string) string {
	var out []string
	in := false
	for _, l := range strings.Split(screen, "\n") {
		_, inner, ok := strings.Cut(l, "┃")
		inner, _, closed := strings.Cut(inner, "┃")
		in = ok && closed && (in || strings.Contains(inner, head))
		if in && !strings.Contains(inner, "history") && !strings.Contains(inner, "cpu") && !strings.Contains(inner, "mem ") {
			out = append(out, inner)
		}
	}
	return strings.Join(out, "\n")
}

// TestPanelNumbersNeverChange renders every layout and checks that each
// panel drawn carries its own number.
func TestPanelNumbersNeverChange(t *testing.T) {
	t.Parallel()

	names := map[string]string{
		"cluster": "1", "nodes": "2", "pods": "3", "workloads": "4", "events": "5", "k3s": "6", "top pods": "7",
	}
	// A label opens a summary line or sits in a box's top edge.
	label := regexp.MustCompile(`(?m)(?:^ {3}|^›  |[┤┫] )(\d) (cluster|nodes|pods|workloads|events|k3s|top pods)\b`)
	m := mount(t, newFeed(busyK3s), wideW, wideH)
	for _, size := range [][2]int{{207, 52}, {160, 40}, {120, 34}, {100, 24}, {80, 18}, {60, 8}, {50, 12}} {
		sized, _ := m.Update(tuitest.Resize(size[0], size[1]))
		screen := tuitest.Screen(sized)
		hits := label.FindAllStringSubmatch(screen, -1)
		if len(hits) == 0 {
			t.Errorf("%dx%d: no panel label found:\n%s", size[0], size[1], screen)
		}
		for _, hit := range hits {
			if names[hit[2]] != hit[1] {
				t.Errorf("%dx%d: %q carries number %s, want %s", size[0], size[1], hit[0], hit[1], names[hit[2]])
			}
		}
	}
}
