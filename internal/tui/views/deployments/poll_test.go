package deployments_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

// watchHint is the label of the w binding in the key hints.
func watchHint(m deployments.Model) string {
	for _, b := range m.Help() {
		if slices.Contains(b.Keys(), "w") {
			return b.Help().Desc
		}
	}
	return ""
}

// TestPollsListFromTheWatchCache: the first load and every tick list with
// ResourceVersion "0"; r asks for a consistent read.
func TestPollsListFromTheWatchCache(t *testing.T) {
	t.Parallel()

	var rec listRecorder
	m, clk := mountedOn(t, tuitest.NewFakeClient(t, rec.option()), tuitest.NamespaceDemo)
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	m, _ = tuitest.Drive(t, m, tick)
	_, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got, want := rec.versions(), []string{"0", "0", ""}; !slices.Equal(got, want) {
		t.Errorf("list ResourceVersions = %q, want %q", got, want)
	}
	if got := clk.Delays(); len(got) == 0 || got[0].Seconds() != 5 {
		t.Errorf("tick delays = %v, want 5s first", got)
	}
}

// TestWatchOffArmsNothing: with w off no tick refreshes, and the w hint
// says so; w again resumes.
func TestWatchOffArmsNothing(t *testing.T) {
	t.Parallel()

	var rec listRecorder
	m, clk := mountedOn(t, tuitest.NewFakeClient(t, rec.option()), tuitest.NamespaceDemo)
	if got := watchHint(m); got != "watch 5s" {
		t.Errorf("hint = %q, want watch 5s", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchHint(m); got != "paused" {
		t.Errorf("hint after w = %q, want paused", got)
	}
	retired, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, retired)
	if got := rec.versions(); len(got) != 1 {
		t.Fatalf("a paused view refreshed: %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("w on armed no tick")
	}
	_, _ = tuitest.Drive(t, m, tick)
	if got := rec.versions(); len(got) != 2 {
		t.Errorf("lists after resuming = %q, want 2", got)
	}
}

// TestSlowAPIStretchesTheWatch: a timed-out list doubles the cadence, and
// the hint names it.
func TestSlowAPIStretchesTheWatch(t *testing.T) {
	t.Parallel()

	timeout := tuitest.WithReactor("list", "deployments", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("list: %w", context.DeadlineExceeded)
	})
	m, clk := mountedOn(t, tuitest.NewFakeClient(t, timeout), tuitest.NamespaceDemo)
	if got := watchHint(m); got != "watch 10s (slow API)" {
		t.Errorf("hint = %q, want watch 10s (slow API)", got)
	}
	if got := clk.Delays(); len(got) != 1 || got[0].Seconds() != 10 {
		t.Errorf("tick delays = %v, want [10s]", got)
	}
}

// TestTickOfAnotherViewIsDropped: every polling view gets every tick; only
// the view that armed it refreshes.
func TestTickOfAnotherViewIsDropped(t *testing.T) {
	t.Parallel()

	var rec listRecorder
	c := tuitest.NewFakeClient(t, rec.option())
	_, clk := mountedOn(t, c, tuitest.NamespaceDemo)
	other := mounted(t, c, tuitest.NamespaceDemo)
	tick, _ := clk.Fire()
	if _, out := tuitest.Drive(t, other, tick); len(out) != 0 {
		t.Errorf("a foreign tick started work:\n%s", tuitest.Describe(out))
	}
	if got := rec.versions(); len(got) != 2 {
		t.Errorf("lists = %q, want the two first loads only", got)
	}
}

func TestHelpBindingsMatchTheirKeys(t *testing.T) {
	t.Parallel()

	m := deployments.New(nil, "")
	for _, b := range m.Help() {
		if !key.Matches(tuitest.PressKey(b.Keys()[0]), b) {
			t.Errorf("binding %q does not match its own key", b.Help().Key)
		}
	}
	if got := watchHint(m); got != "paused" {
		t.Errorf("hint without a client = %q, want paused", got)
	}
}
