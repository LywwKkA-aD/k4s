package services_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

// watchLabel is the help text of the w binding.
func watchLabel(t *testing.T, m services.Model) string {
	t.Helper()
	for _, b := range m.Help() {
		if slices.Contains(b.Keys(), "w") {
			return b.Help().Desc
		}
	}
	t.Fatal("no w binding")
	return ""
}

// TestPollsReadTheWatchCacheAndRefreshReadsThrough (M4.4): the first load
// and every tick list with ResourceVersion "0", r with "".
func TestPollsReadTheWatchCacheAndRefreshReadsThrough(t *testing.T) {
	t.Parallel()

	var log rvLog
	m, clk := clocked(tuitest.NewFakeClient(t, log.reactor()), tuitest.NamespaceDemo)
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
	m, clk := clocked(tuitest.NewFakeClient(t, log.reactor()), tuitest.NamespaceDemo)
	m, _ = tuitest.Mount(t, m, width, height)
	if got := watchLabel(t, m); got != "watch 5s" {
		t.Fatalf("w label = %q, want %q", got, "watch 5s")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchLabel(t, m); got != "paused" {
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

func TestWatchLabelShowsTheBackoff(t *testing.T) {
	t.Parallel()

	slow := tuitest.WithReactor("list", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("slow: %w", context.DeadlineExceeded)
	})
	m := mounted(t, tuitest.NewFakeClient(t, slow), tuitest.NamespaceDemo)
	if got, want := watchLabel(t, m), "watch 10s (slow API)"; got != want {
		t.Fatalf("w label = %q, want %q", got, want)
	}
}

func TestNoClientArmsNothing(t *testing.T) {
	t.Parallel()

	m, clk := clocked(nil, "")
	m, out := tuitest.Mount(t, m, width, height)
	if len(out) != 0 || clk.Pending() != 0 {
		t.Fatalf("a view without a client scheduled work: %s, %d timers", tuitest.Describe(out), clk.Pending())
	}
	if got := watchLabel(t, m); got != "paused" {
		t.Fatalf("w label = %q, want paused", got)
	}
}

// TestHelpLabelsFollowTheBindings guards the keys every footer golden
// shows; the w label is the poller's.
func TestHelpLabelsFollowTheBindings(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	got := make([]string, 0, len(m.Help()))
	for _, b := range m.Help() {
		got = append(got, b.Help().Key+" "+b.Help().Desc)
	}
	want := []string{"enter describe", "l tail all pods", "f port-forward", "/ filter", "w watch 5s", "r refresh"}
	if !slices.Equal(got, want) {
		t.Fatalf("help = %q, want %q", got, want)
	}
}
