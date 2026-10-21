package dashboard_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestZoom: z draws the focused panel over the whole body, with every row;
// Esc leaves the zoom first and keeps the focus; z again also leaves it.
func TestZoom(t *testing.T) {
	t.Parallel()

	m, _ := press(t, mount(t, newFeed(busyK3s), 120, 34), "3", "z")
	screen := tuitest.Screen(m)
	requireAll(t, screen, []string{"3 pods", "ingest-6d8c7b5f9-m2kzp", "report-28741230-9q8wd"}, []string{"1 cluster ├", "4 workloads ├", "+3 · "})
	requireSize(t, screen, 120, 34)
	if got := hint(m, "z"); got != "back" {
		t.Fatalf("z hint while zoomed = %q", got)
	}
	back, _, used := m.Back()
	if !used || !strings.Contains(tuitest.Screen(back), "1 cluster") || back.Command().Display() != podsCmd {
		t.Fatalf("Esc in zoom: used %v, command %q", used, back.Command().Display())
	}
	m, _ = press(t, m, "z")
	if !strings.Contains(tuitest.Screen(m), "1 cluster") {
		t.Fatal("z again did not leave the zoom")
	}
	if got := hint(m, "z"); got != "zoom" {
		t.Fatalf("z hint on a panel = %q", got)
	}
	_, out := press(t, m, "0", "z")
	if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "z zooms") {
		t.Fatalf("z on the banner:\n%s", tuitest.Describe(out))
	}
}

// TestHiddenPanelOpensZoomed: a panel this size leaves out opens zoomed
// on its number, and pod metrics are polled while it shows.
func TestHiddenPanelOpensZoomed(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m, _ := press(t, mount(t, f, 120, 34), "7")
	requireAll(t, tuitest.Screen(m), []string{"7 top pods", "monitoring/prometheus-k8s-0"}, []string{"3 pods"})
	if got := command(m); got != "kubectl top pods -A --sort-by=cpu" {
		t.Fatalf("command = %q", got)
	}
	before := len(f.calls())
	m = advance(t, m, f, 15*time.Second)
	asked := ""
	for _, p := range f.calls()[before:] {
		asked += dueOf(p) + " "
	}
	if !strings.Contains(asked, dash.PodMetrics.String()) {
		t.Fatalf("with top pods zoomed the polls asked for %s", asked)
	}
	back, _, _ := m.Back()
	if strings.Contains(tuitest.Screen(back), "7 top pods") {
		t.Fatal("Esc left panel 7 on screen at a size that hides it")
	}
}

// calm is the busy cluster before payments-worker r8vqn crashed and its
// deployment lost its pods.
func calm() dash.Snapshot {
	s := busyK3s()
	s.Pods.Problems = append(s.Pods.Problems[:1:1], s.Pods.Problems[2:]...)
	s.Workloads.Rows = s.Workloads.Rows[1:]
	return s
}

// TestChangeRing: what changed between two polls goes into the ≈ seen by
// k4s ring under the warnings, newest first; the first snapshot adds
// nothing, and a row that leaves the problems reads as cleared.
func TestChangeRing(t *testing.T) {
	t.Parallel()

	f := newFeed(calm)
	m := mount(t, f, wideW, wideH)
	if strings.Contains(tuitest.Screen(m), "SEEN BY K4S") {
		t.Fatal("the first snapshot filled the ring")
	}
	f.set(busyK3s)
	m = advance(t, m, f, 5*time.Second)
	requireAll(t, tuitest.Screen(m), []string{
		"≈ SEEN BY K4S  state changes between two polls, kept in memory for this session",
		"▲ 14:02  pod shop/payments-worker-6c9f7d8b5-r8vqn",
		"ok → CrashLoopBackOff",
		"▲ 14:02  deploy shop/payments-worker",
		"ready → 0/2",
	}, nil)
	f.set(calm)
	m = advance(t, m, f, 60*time.Second)
	requireAll(t, tuitest.Screen(m), []string{
		"● 14:02  pod shop/payments-worker-6c9f7d8b5-r8vqn  CrashLoopBackOff → cleared",
		"● 14:02  deploy shop/payments-worker               0/2 → ready",
	}, nil)

	// The ring's rows are cursor stops that describe their object.
	m, _ = press(t, m, "5")
	for range 12 {
		m, _ = press(t, m, "down")
	}
	if got := command(m); !strings.HasPrefix(got, "kubectl describe ") {
		t.Fatalf("command on a ring row = %q", got)
	}
}

// TestToastInTheBanner: in the wide layout a change shows in the banner's
// toast lane, the newest with +N for the rest, and one 5s tick dismisses
// it. The first snapshot shows none.
func TestToastInTheBanner(t *testing.T) {
	t.Parallel()

	f := newFeed(calm)
	m := mount(t, f, wideW, wideH)
	if f.frames.Pending() != 0 || strings.Contains(bannerLine(t, tuitest.Screen(m), 1), "→") {
		t.Fatal("the first snapshot raised a toast")
	}
	f.set(busyK3s)
	m = advance(t, m, f, 5*time.Second)
	line := bannerLine(t, tuitest.Screen(m), 1)
	if !strings.HasSuffix(line, "▲ pod shop/payments-worker-6c9f7d8b5-r8vqn → CrashLoopBackOff  +1 ") {
		t.Fatalf("toast lane = %q", line)
	}
	if d := f.frames.Delays(); f.frames.Pending() != 1 || d[len(d)-1] != 5*time.Second {
		t.Fatalf("toast armed %d ticks, delays %v; want one of 5s", f.frames.Pending(), d)
	}
	msg, _ := f.frames.Fire()
	m, _ = tuitest.Drive(t, m, msg)
	if strings.Contains(bannerLine(t, tuitest.Screen(m), 1), "→ CrashLoopBackOff") {
		t.Fatal("the toast outlived its tick")
	}
}

// TestToastInTheStatusLine: outside the wide layout the toast is a status
// message, and the root's status line times it out.
func TestToastInTheStatusLine(t *testing.T) {
	t.Parallel()

	f := newFeed(calm)
	m := mount(t, f, 120, 34)
	f.set(busyK3s)
	var out []tea.Msg
	m, out = tuitest.Drive(t, m, f.clock.Advance(tickStep)...)
	msg, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || msg.Text != "▲ pod shop/payments-worker-6c9f7d8b5-r8vqn → CrashLoopBackOff · +1" {
		t.Fatalf("status toast:\n%s", tuitest.Describe(out))
	}
	if f.frames.Pending() != 0 {
		t.Fatal("a status toast armed a tick of its own")
	}
	_ = m
}

// TestRingInZoomedEvents: the accordion has no room for the ring; z on
// panel 5 shows it.
func TestRingInZoomedEvents(t *testing.T) {
	t.Parallel()

	f := newFeed(calm)
	m := mount(t, f, 80, 24)
	f.set(busyK3s)
	m = advance(t, m, f, 5*time.Second)
	if strings.Contains(tuitest.Screen(m), "SEEN BY K4S") {
		t.Fatal("the accordion drew the ring")
	}
	m, _ = press(t, m, "5", "z")
	requireAll(t, tuitest.Screen(m), []string{"SEEN BY K4S", "pod shop/payments-worker-6c9f7d8b5-r8vqn"}, nil)
}
