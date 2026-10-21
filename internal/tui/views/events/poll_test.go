package events_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestPollsReadTheWatchCacheAndRefreshReadsThrough (M4.4): the first load
// and every tick list with ResourceVersion "0", r with "".
func TestPollsReadTheWatchCacheAndRefreshReadsThrough(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, clk := clocked(client(t, log.reactor()), demo)
	m, _ = tuitest.Mount(t, m, width, height)
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	m, _ = tuitest.Drive(t, m, tick)
	_, _ = tuitest.Drive(t, m, tuitest.Press("r"))

	if got, want := log.seen(), []string{"0", "0", ""}; !slices.Equal(got, want) {
		t.Fatalf("list resource versions = %q, want %q", got, want)
	}
}

func TestWatchToggleStopsTheTicks(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, clk := clocked(client(t, log.reactor()), demo)
	m, _ = tuitest.Mount(t, m, width, height)
	if got := helpLabel(t, m, "w"); got != "watch 5s" {
		t.Fatalf("w label = %q, want watch 5s", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := helpLabel(t, m, "w"); got != "paused" {
		t.Fatalf("w label after w = %q, want paused", got)
	}
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := log.seen(); len(got) != 1 {
		t.Fatalf("a paused view listed on its old tick: %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("w back on armed no tick")
	}
	_, _ = tuitest.Drive(t, m, tick)
	if got := log.seen(); len(got) != 2 {
		t.Fatalf("lists after watch came back = %q, want 2", got)
	}
}

func TestRefreshWhileListingSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := clocked(client(t), demo)
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	requireStatus(t, out, status.Info, "refresh already running")
}

// TestFailedRefreshKeepsTheRows: the rows of the last good list stay
// under a stale banner, and the error is said once.
func TestFailedRefreshKeepsTheRows(t *testing.T) {
	t.Parallel()

	var down tuitest.Outage
	m, clk := clocked(client(t, down.Lists("events", errors.New("connection refused"))), demo)
	m, _ = tuitest.Mount(t, m, width, height)
	down.Down()
	tick, _ := clk.Fire()
	m, out := tuitest.Drive(t, m, tick)
	requireStatus(t, out, status.Error, "list events: connection refused")
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "stale: connection refused") || !strings.Contains(screen, "BackOff") {
		t.Errorf("stale view:\n%s", screen)
	}
	tick, _ = clk.Fire()
	_, out = tuitest.Drive(t, m, tick)
	if said := tuitest.All[views.StatusMsg](out); len(said) != 0 {
		t.Errorf("the same error was said twice:\n%s", tuitest.Describe(out))
	}
}

func TestFirstListFailing(t *testing.T) {
	t.Parallel()

	failing := tuitest.WithReactor("list", "events", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("slow: %w", context.DeadlineExceeded)
	})
	m, _ := mounted(t, client(t, failing), demo)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "events unavailable: list events: slow: context deadline exceeded") {
		t.Errorf("screen:\n%s", screen)
	}
	if got := helpLabel(t, m, "w"); got != "watch 10s (slow API)" {
		t.Errorf("w label = %q", got)
	}
	_, out := tuitest.Drive(t, m, tuitest.Enter())
	requireStatus(t, out, status.Warn, "events unavailable, press r")
}
