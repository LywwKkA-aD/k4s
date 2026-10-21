package pods_test

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestListResourceVersions: the first load and every tick may read the
// apiserver's watch cache (RV "0"); r asks for a consistent read (M4.4).
func TestListResourceVersions(t *testing.T) {
	t.Parallel()

	var log rvLog
	clk := asynctest.NewClock(epoch)
	m := mountedAt(t, tuitest.NewFakeClient(t, log.option()), tuitest.NamespaceDemo, clk)
	if got, want := log.all(), []string{"0"}; !slices.Equal(got, want) {
		t.Fatalf("first load RVs = %q, want %q", got, want)
	}

	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	m, _ = tuitest.Drive(t, m, tick)
	if got, want := log.all(), []string{"0", "0"}; !slices.Equal(got, want) {
		t.Fatalf("after a tick RVs = %q, want %q", got, want)
	}

	_, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got, want := log.all(), []string{"0", "0", ""}; !slices.Equal(got, want) {
		t.Fatalf("after r RVs = %q, want %q", got, want)
	}
}

// TestWatchLabelsTheBindingAndStopsTicks: w pauses and resumes polling, and
// its key hint says which; a paused view arms no tick.
func TestWatchLabelsTheBindingAndStopsTicks(t *testing.T) {
	t.Parallel()

	clk := asynctest.NewClock(epoch)
	m := mountedAt(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo, clk)
	if got := watchHint(m); got != "watch 5s" {
		t.Errorf("w hint = %q, want %q", got, "watch 5s")
	}
	if got := clk.Delays(); !slices.Equal(got, []time.Duration{5 * time.Second}) {
		t.Errorf("armed delays = %v, want one 5s tick", got)
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchHint(m); got != "paused" {
		t.Errorf("w hint after w = %q, want paused", got)
	}
	tick, _ := clk.Fire()
	if _, out := tuitest.Step(t, m, tick); len(out) != 0 {
		t.Errorf("a tick armed before the pause fetched:\n%s", tuitest.Describe(out))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if clk.Pending() != 0 {
		t.Errorf("a paused view armed %d ticks after r", clk.Pending())
	}

	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchHint(m); got != "watch 5s" || clk.Pending() != 1 {
		t.Errorf("w again: hint %q, %d ticks pending; want watch 5s and one tick", got, clk.Pending())
	}
}

// TestSlowAPIBacksOff: a list that times out doubles the next interval, and
// the w hint shows it.
func TestSlowAPIBacksOff(t *testing.T) {
	t.Parallel()

	slow := tuitest.WithReactor("list", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list: %w", context.DeadlineExceeded)
	})
	clk := asynctest.NewClock(epoch)
	m := mountedAt(t, tuitest.NewFakeClient(t, slow), tuitest.NamespaceDemo, clk)
	if got, want := watchHint(m), "watch 10s (slow API)"; got != want {
		t.Errorf("w hint = %q, want %q", got, want)
	}
	if got := clk.Delays(); !slices.Equal(got, []time.Duration{10 * time.Second}) {
		t.Errorf("armed delays = %v, want 10s", got)
	}
}

// TestTickOfAnotherViewIsDropped: every polling view receives the same tick
// type, so a view takes only the ticks it armed itself.
func TestTickOfAnotherViewIsDropped(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	clk := asynctest.NewClock(epoch)
	_ = mountedAt(t, c, tuitest.NamespaceDemo, clk)
	other := mountedAt(t, c, tuitest.NamespaceDemo, asynctest.NewClock(epoch))

	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("no tick armed")
	}
	if _, isTick := tick.(async.TickMsg); !isTick {
		t.Fatalf("the clock fired %T", tick)
	}
	if _, out := tuitest.Step(t, other, tick); len(out) != 0 {
		t.Errorf("another view's tick started a fetch:\n%s", tuitest.Describe(out))
	}
}
