package dashboard_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

func TestWithoutAClientNothingIsScheduled(t *testing.T) {
	t.Parallel()

	m := dashboard.New(nil, nil)
	if cmd := m.Init(); cmd != nil {
		t.Fatal("Init without a client must schedule nothing")
	}
	_, out := tuitest.Drive(t, m, tuitest.Press("r"))
	if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || !strings.Contains(msg.Text, "not connected") {
		t.Fatalf("r without a client should say so:\n%s", tuitest.Describe(out))
	}
	if !m.Command().IsZero() {
		t.Fatalf("command without a client = %q, want none", m.Command().Display())
	}
}

func TestTheFirstRefreshArmsOneTick(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	mount(t, f, wideW, wideH)
	if n := len(f.calls()); n != 1 {
		t.Fatalf("refreshes after Init = %d, want 1", n)
	}
	if d := f.clock.Delays(); len(d) != 1 || d[0] != 5*time.Second {
		t.Fatalf("ticks armed = %v, want one at 5s", d)
	}
}

func TestWatchOffSchedulesNothing(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	c := tuitest.NewFakeClient(t)
	m := mountOn(t, c, f, wideW, wideH)
	m, _ = press(t, m, "w")
	if got := hint(m, "w"); got != "paused" {
		t.Fatalf("w hint = %q, want paused", got)
	}
	m = advance(t, m, f, time.Minute)
	if n := len(f.calls()); n != 1 {
		t.Fatalf("refreshes with watch off = %d, want only the first", n)
	}
	if f.clock.Pending() != 0 {
		t.Fatalf("%d ticks still pending with watch off", f.clock.Pending())
	}

	// Paused survives a visit elsewhere: the next dashboard does not poll.
	next := dashboard.New(c, nil, append(f.options(), dashboard.WithMemory(m.Memory()))...)
	next, _ = tuitest.Mount(t, next, wideW, wideH)
	if got := hint(next, "w"); got != "paused" || f.clock.Pending() != 0 {
		t.Fatalf("after a visit: hint %q, pending %d; want paused and none", got, f.clock.Pending())
	}

	m, _ = press(t, m, "w")
	if got := hint(m, "w"); got != "watch 5s" || f.clock.Pending() != 1 {
		t.Fatalf("w again: hint %q, pending %d; want watch 5s and one tick", got, f.clock.Pending())
	}
}

func TestTicksRefreshTheFastTier(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	advance(t, m, f, time.Minute)
	plans := f.calls()
	if len(plans) != 13 {
		t.Fatalf("refreshes in the first minute = %d, want 13 (one at 0 and every 5s)", len(plans))
	}
	if due := dueOf(plans[1]); !strings.Contains(due, "pods") || strings.Contains(due, "nodes") {
		t.Errorf("a 5s tick asked for %s, want the fast tier only", due)
	}
}

func TestBlurSlowsAndFocusRestoresTheCadence(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	m, _ = tuitest.Drive(t, m, tuitest.Blur())
	if got := hint(m, "w"); got != "watch 15s" {
		t.Fatalf("blurred hint = %q, want watch 15s", got)
	}
	m = advance(t, m, f, 45*time.Second)
	if got := podsListedAt(f.calls()); got != "[0s 15s 30s 45s]" {
		t.Fatalf("pods listed at %s while blurred, want every 15s", got)
	}
	if d := f.clock.Delays(); d[len(d)-1] != 15*time.Second {
		t.Fatalf("delays %v, want the last tick 15s out", d)
	}

	before := len(f.clock.Delays())
	m, _ = tuitest.Drive(t, m, tuitest.Focus())
	delays := f.clock.Delays()
	if got := hint(m, "w"); got != "watch 5s" || len(delays) != before+1 || delays[len(delays)-1] > 5*time.Second {
		t.Fatalf("focus regained: hint %q, delays %v; want watch 5s and a tick re-armed within 5s", got, delays)
	}
}

func TestIdleStretchesTheCadenceUntilAKey(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	m = advance(t, m, f, 11*time.Minute)
	if got := hint(m, "w"); got != "watch 15s" {
		t.Fatalf("hint after 11 idle minutes = %q, want watch 15s", got)
	}
	m, _ = press(t, m, "tab")
	if got := hint(m, "w"); got != "watch 5s" {
		t.Fatalf("hint after a key = %q, want watch 5s", got)
	}
}

func TestRefreshKeyForcesEveryTier(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	busy, _ := tuitest.Step(t, m, tuitest.Press("r"))
	plans := f.calls()
	if last := plans[len(plans)-1]; !last.Force || len(last.Due) < 15 {
		t.Fatalf("r planned force=%v with %d requests, want every tier", last.Force, len(last.Due))
	}
	if got := tuitest.Screen(busy); !strings.Contains(got, "refreshing…") {
		t.Fatalf("r in flight should show refreshing:\n%s", got)
	}
	_, out := tuitest.Step(t, busy, tuitest.Press("r"))
	if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || msg.Text != "refresh already running" {
		t.Fatalf("r while one runs:\n%s", tuitest.Describe(out))
	}
	if len(f.calls()) != len(plans) {
		t.Fatal("r while one runs started a second refresh")
	}
}

func TestAnotherDashboardsReplyIsDropped(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	a := mount(t, f, wideW, wideH)
	b := mount(t, newFeed(allClear), wideW, wideH)
	_, out := tuitest.Step(t, a, tuitest.Press("r"))
	before := tuitest.Frame(b)
	b, more := tuitest.Drive(t, b, out...)
	if tuitest.Frame(b) != before || len(more) != 0 {
		t.Fatalf("b took a's reply:\n%s", tuitest.Describe(more))
	}
}

// TestEqualSnapshotsRenderTheSame is an idle guard: a poll that brings
// nothing new repaints byte for byte, and so does one that changes only
// what is not drawn (the request and byte counts, which no section hash
// covers).
func TestEqualSnapshotsRenderTheSame(t *testing.T) {
	t.Parallel()

	n := 0
	f := newFeed(func() dash.Snapshot {
		n++
		s := busyK3s()
		s.Requests, s.Bytes = n, n*1000
		return s
	})
	m := mount(t, f, wideW, wideH)
	first := tuitest.Frame(m)
	m = advance(t, m, f, 5*time.Second)
	if len(f.calls()) != 2 {
		t.Fatalf("refreshes = %d, want 2", len(f.calls()))
	}
	if tuitest.Frame(m) != first {
		t.Fatalf("an equal poll changed the view:\n%s\n---\n%s", tuitest.Strip(first), tuitest.Screen(m))
	}
}

// podsListedAt is the plan times of the refreshes that listed pods.
func podsListedAt(plans []dash.Plan) string {
	var at []time.Duration
	for _, p := range plans {
		for _, r := range p.Due {
			if r.Endpoint == dash.Pods {
				at = append(at, p.Now)
			}
		}
	}
	return fmt.Sprint(at)
}

func dueOf(p dash.Plan) string {
	names := make([]string, 0, len(p.Due))
	for _, r := range p.Due {
		names = append(names, r.Endpoint.String())
	}
	return strings.Join(names, ",")
}

// hint is the help text of the binding k in m's footer hints.
func hint(m dashboard.Model, k string) string {
	for _, b := range m.Help() {
		if b.Help().Key == k {
			return b.Help().Desc
		}
	}
	return ""
}
