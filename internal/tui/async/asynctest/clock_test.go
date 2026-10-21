package asynctest_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
)

var _ async.Clock = (*asynctest.Clock)(nil)

var start = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

type fired struct {
	name string
	at   time.Time
}

func named(name string) func(time.Time) tea.Msg {
	return func(at time.Time) tea.Msg { return fired{name: name, at: at} }
}

func TestTickRegistersOnlyWhenTheCmdRuns(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(start)

	cmd := clk.Tick(5*time.Second, named("a"))
	if got := clk.Pending(); got != 0 {
		t.Fatalf("Pending before the Cmd ran = %d, want 0", got)
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("tick Cmd returned %v, want nil so a driver never waits on it", msg)
	}
	if got := clk.Pending(); got != 1 {
		t.Fatalf("Pending after the Cmd ran = %d, want 1", got)
	}
	if got, want := clk.Delays(), []time.Duration{5 * time.Second}; !slices.Equal(got, want) {
		t.Fatalf("Delays = %v, want %v", got, want)
	}
}

func TestFireDeliversTheEarliestTimer(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(start)
	clk.Tick(10*time.Second, named("late"))()
	clk.Tick(5*time.Second, named("early"))()
	clk.Tick(5*time.Second, named("early-second"))()

	want := []fired{
		{name: "early", at: start.Add(5 * time.Second)},
		{name: "early-second", at: start.Add(5 * time.Second)},
		{name: "late", at: start.Add(10 * time.Second)},
	}
	for _, w := range want {
		msg, ok := clk.Fire()
		if !ok {
			t.Fatalf("Fire found nothing, want %q", w.name)
		}
		if msg != w {
			t.Fatalf("Fire = %v, want %v", msg, w)
		}
		if !clk.Now().Equal(w.at) {
			t.Fatalf("Now = %v after firing %q, want %v", clk.Now(), w.name, w.at)
		}
	}
	if msg, ok := clk.Fire(); ok {
		t.Fatalf("Fire on an empty clock = %v, want nothing", msg)
	}
}

func TestAdvanceFiresOnlyDueTimers(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(start)
	clk.Tick(20*time.Second, named("c"))()
	clk.Tick(5*time.Second, named("a"))()
	clk.Tick(10*time.Second, named("b"))()

	got := clk.Advance(10 * time.Second)
	want := []tea.Msg{
		fired{name: "a", at: start.Add(5 * time.Second)},
		fired{name: "b", at: start.Add(10 * time.Second)},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Advance(10s) = %v, want %v", got, want)
	}
	if clk.Pending() != 1 {
		t.Fatalf("Pending = %d, want 1 (c is not due)", clk.Pending())
	}
	if !clk.Now().Equal(start.Add(10 * time.Second)) {
		t.Fatalf("Now = %v, want start+10s", clk.Now())
	}
	if got := clk.Advance(time.Second); len(got) != 0 {
		t.Fatalf("Advance(1s) = %v, want nothing", got)
	}
}

func TestDelaysIsACopy(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(start)
	clk.Tick(time.Second, named("a"))()
	got := clk.Delays()
	got[0] = time.Hour
	if clk.Delays()[0] != time.Second {
		t.Fatal("writing to the Delays result changed the clock")
	}
}

func TestZeroClockIsUsable(t *testing.T) {
	t.Parallel()
	var clk asynctest.Clock
	clk.Tick(time.Second, named("a"))()
	msg, ok := clk.Fire()
	if !ok || msg != (fired{name: "a", at: time.Time{}.Add(time.Second)}) {
		t.Fatalf("Fire = %v, %v", msg, ok)
	}
}

func TestConcurrentTicks(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(start)
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() { clk.Tick(time.Second, named("x"))() })
	}
	wg.Wait()
	if got := len(clk.Advance(time.Second)); got != 32 {
		t.Fatalf("fired %d timers, want 32", got)
	}
}
