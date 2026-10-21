package dashboard_test

import (
	"context"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// feed answers refreshes with a fixture and stamps it the way dash.Fetcher
// does: wall time from the test clock, and a ledger entry at the plan's
// time for every endpoint the plan made due. vary moves the meters and the
// not-ok count a little on every answer, so history has a shape.
type feed struct {
	mu    sync.Mutex
	clock *asynctest.Clock
	// frames is the motion clock, apart from the poll's so that counting
	// ticks never counts frames.
	frames *asynctest.Clock
	base   func() dash.Snapshot
	vary   bool
	plans  []dash.Plan
}

func newFeed(base func() dash.Snapshot) *feed {
	return &feed{clock: asynctest.NewClock(start), frames: asynctest.NewClock(start), base: base}
}

func (f *feed) fetch(_ context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.plans = append(f.plans, p)
	s := f.base()
	s.Now, s.At = p.Now, f.clock.Now()
	ledger := prev.Ledger
	for _, r := range p.Due {
		u := s.Ledger[r.Endpoint]
		u.Fetched, u.At = true, p.Now
		ledger[r.Endpoint] = u
	}
	s.Ledger = ledger
	if f.vary {
		wobble(&s, len(f.plans))
	}
	s.Hash = hashes(s)
	return s
}

// hashes sums each section as dash does, so a fixture that changes what a
// panel shows changes its hash, and one that does not keeps it.
func hashes(s dash.Snapshot) dash.Hashes {
	sum := func(v any) uint64 {
		h := fnv.New64a()
		_, _ = fmt.Fprintf(h, "%+v", v)
		return h.Sum64()
	}
	return dash.Hashes{
		Cluster: sum(s.Cluster), Nodes: sum(s.Nodes), Pods: sum(s.Pods), Workloads: sum(s.Workloads),
		Events: sum(s.Events), K3s: sum(s.K3s), TopPods: sum(s.TopPods), Verdict: sum(s.Verdict), Next: sum(s.Next),
	}
}

// set makes base the fixture of the next answers.
func (f *feed) set(base func() dash.Snapshot) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.base = base
}

// calls returns the plans of every refresh that ran, in order.
func (f *feed) calls() []dash.Plan {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.plans)
}

// wobble moves the meters and the not-ok count along a fixed pattern.
func wobble(s *dash.Snapshot, n int) {
	shift := func(m dash.Meter, by int64) dash.Meter {
		m.Used = max(m.Used+m.Capacity*by/100, 0)
		m.Percent = int(m.Used * 100 / m.Capacity)
		return m
	}
	steps := []int64{0, 3, 7, 4, -2, -6, -3, 2, 9, 5}
	by := steps[n%len(steps)]
	s.Nodes.CPU = shift(s.Nodes.CPU, by)
	s.Nodes.Memory = shift(s.Nodes.Memory, by/2)
	for i := range s.Nodes.Rows {
		s.Nodes.Rows[i].CPU = shift(s.Nodes.Rows[i].CPU, by*int64(i+1)/2)
	}
	s.Pods.NotOK += n % 4
}

func (f *feed) options() []dashboard.Option {
	return []dashboard.Option{
		dashboard.WithFetch(f.fetch), dashboard.WithClock(f.clock.Now), dashboard.WithTicker(f.clock),
		dashboard.WithFrames(f.frames),
	}
}

// maxFrames bounds settle: no event may run longer than this many frames.
const maxFrames = 64

// settle fires the motion clock until nothing moves and returns the
// frames it fired: the animation frames and the spinner's delay. A clock
// still running after maxFrames fails the test.
func (f *feed) settle(t testing.TB, m dashboard.Model) (dashboard.Model, []tea.Msg) {
	t.Helper()
	var fired []tea.Msg
	for f.frames.Pending() > 0 {
		if len(fired) >= maxFrames {
			t.Fatalf("the motion clock still runs after %d frames", maxFrames)
		}
		msg, _ := f.frames.Fire()
		fired = append(fired, msg)
		m, _ = tuitest.Drive(t, m, msg)
	}
	return m, fired
}

// framesIn counts the animation frames among msgs.
func framesIn(msgs []tea.Msg) int {
	n := 0
	for _, msg := range msgs {
		if fmt.Sprintf("%T", msg) == "dashboard.animFrameMsg" {
			n++
		}
	}
	return n
}

// mount builds the dashboard over f on a fake client, sized w x h, runs
// its first refresh and lets the motion it starts come to rest.
func mount(t testing.TB, f *feed, w, h int) dashboard.Model {
	t.Helper()
	return mountOn(t, tuitest.NewFakeClient(t), f, w, h)
}

// mountOn is mount on client c.
func mountOn(t testing.TB, c *k8s.Client, f *feed, w, h int) dashboard.Model {
	t.Helper()
	m := dashboard.New(c, nil, f.options()...)
	m, _ = tuitest.Mount(t, m, w, h)
	m, _ = f.settle(t, m)
	return m
}

// press delivers keys one at a time and returns every message they caused.
func press(t testing.TB, m dashboard.Model, keys ...string) (dashboard.Model, []tea.Msg) {
	t.Helper()
	return tuitest.Drive(t, m, tuitest.Keys(keys...)...)
}

// command is the footer command m shows, as displayed.
func command(m dashboard.Model) string { return m.Command().Display() }

// tickStep is the poller's base cadence: advance walks the clock by it.
const tickStep = 5 * time.Second

// advance moves the clock on by d in tickStep steps, delivering every tick
// that comes due on the way and what it starts.
func advance(t testing.TB, m dashboard.Model, f *feed, d time.Duration) dashboard.Model {
	step := tickStep
	t.Helper()
	for done := time.Duration(0); done < d; done += step {
		m, _ = tuitest.Drive(t, m, f.clock.Advance(step)...)
	}
	return m
}

// requireSize fails unless every line of view is exactly w columns and
// there are exactly h lines.
func requireSize(t *testing.T, view string, w, h int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != h {
		t.Fatalf("%dx%d: %d lines, want %d", w, h, len(lines), h)
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != w {
			t.Fatalf("%dx%d: line %d is %d wide, want %d:\n%q", w, h, i, got, w, ansi.Strip(l))
		}
	}
}
