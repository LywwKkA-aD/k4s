package namespaces_test

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
)

// lists records the ResourceVersion of every namespaces list the fake API
// answers, in order.
type lists struct {
	mu  sync.Mutex
	rvs []string
}

func (l *lists) react(a k8stesting.Action) (bool, runtime.Object, error) {
	if la, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
		l.mu.Lock()
		l.rvs = append(l.rvs, la.GetListOptions().ResourceVersion)
		l.mu.Unlock()
	}
	return false, nil, nil
}

func (l *lists) seen() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.rvs)
}

// recorded returns a fake client that records its namespace lists.
func recorded(t *testing.T) (*k8s.Client, *lists) {
	t.Helper()
	l := &lists{}
	return tuitest.NewFakeClient(t, tuitest.WithReactor("list", "namespaces", l.react)), l
}

// watchHint is the label of the w binding: the poller's cadence.
func watchHint(t *testing.T, m namespaces.Model) string {
	t.Helper()
	for _, b := range m.Help() {
		if b.Help().Key == "w" {
			return b.Help().Desc
		}
	}
	t.Fatal("no w binding in Help")
	return ""
}

func TestTicksReadTheCacheAndRefreshReadsConsistently(t *testing.T) {
	t.Parallel()

	c, l := recorded(t)
	m, clk := clocked(t, c)
	tick, ok := clk.Fire()
	if !ok {
		t.Fatal("the first reply armed no tick")
	}
	m, _ = tuitest.Drive(t, m, tick, tuitest.Press("r"))
	if got, want := l.seen(), []string{"0", "0", ""}; !slices.Equal(got, want) {
		t.Errorf("ResourceVersions (load, tick, r) = %q, want %q", got, want)
	}
	if got, want := clk.Delays(), []time.Duration{5 * time.Second, 5 * time.Second, 5 * time.Second}; !slices.Equal(got, want) {
		t.Errorf("tick delays = %v, want %v", got, want)
	}
	if !strings.Contains(tuitest.Screen(m), tuitest.NamespaceDemo) {
		t.Errorf("rows after the refresh:\n%s", tuitest.Screen(m))
	}
}

func TestWatchPausesTheTicks(t *testing.T) {
	t.Parallel()

	c, l := recorded(t)
	m, clk := clocked(t, c)
	if got := watchHint(t, m); got != "watch 5s" {
		t.Errorf("hint while watching = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchHint(t, m); got != "paused" {
		t.Errorf("hint after w = %q", got)
	}
	tick, _ := clk.Fire()
	m, out := tuitest.Drive(t, m, tick)
	if len(out) != 0 || len(l.seen()) != 1 {
		t.Fatalf("a tick armed before w listed again: %q\n%s", l.seen(), tuitest.Describe(out))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	if got := watchHint(t, m); got != "watch 5s" || clk.Pending() != 1 {
		t.Fatalf("w again: hint %q, %d ticks pending, want watch 5s and one", got, clk.Pending())
	}
	tick, _ = clk.Fire()
	_, _ = tuitest.Drive(t, m, tick)
	if len(l.seen()) != 2 {
		t.Errorf("the re-armed tick did not list: %q", l.seen())
	}
}

func TestSlowAPIStretchesTheCadence(t *testing.T) {
	t.Parallel()

	slow := tuitest.WithReactor("list", "namespaces", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, context.DeadlineExceeded
	})
	m, clk := clocked(t, tuitest.NewFakeClient(t, slow))
	if got := watchHint(t, m); got != "watch 10s (slow API)" {
		t.Errorf("hint after a timeout = %q", got)
	}
	if got := clk.Delays(); !slices.Equal(got, []time.Duration{10 * time.Second}) {
		t.Errorf("tick delays = %v, want [10s]", got)
	}
	if !strings.Contains(tuitest.Screen(m), "namespaces unavailable: list namespaces: context deadline exceeded") {
		t.Errorf("screen:\n%s", tuitest.Screen(m))
	}
}

func TestNoClientNeverPolls(t *testing.T) {
	t.Parallel()

	clk := asynctest.NewClock(time.Time{})
	m := namespaces.New(nil, namespaces.WithClock(clk))
	if cmd := m.Init(); cmd != nil {
		t.Error("Init without a client returned a Cmd")
	}
	m, out := tuitest.Drive(t, m, tuitest.Press("w"), tuitest.Press("r"))
	if len(out) != 0 || clk.Pending() != 0 || watchHint(t, m) != "paused" {
		t.Errorf("no client: out=%s pending=%d hint=%q", tuitest.Describe(out), clk.Pending(), watchHint(t, m))
	}
}
