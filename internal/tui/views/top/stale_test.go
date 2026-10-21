package top_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// metricsDown is what the metrics outage says in the status row.
const metricsDown = "metrics unavailable: metrics-server is not ready (503)"

// TestFailedRefreshKeepsTheRowsAndSaysSo: the last good rows stay under a
// stale banner, the failure is a status once per text, and recovery drops
// the banner.
func TestFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m, clk := clocked(t, tuitest.NewFakeClient(t, o.Metrics()), tuitest.NamespaceDemo)

	o.Down()
	tick, _ := clk.Fire()
	m, out := tuitest.Drive(t, m, tick)
	screen := tuitest.Screen(m)
	for _, want := range []string{"stale:", "r retry", tuitest.PodMulti, tuitest.PodWeb1} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "metrics unavailable") || strings.Contains(screen, "install metrics-server") {
		t.Errorf("the error replaced the table:\n%s", screen)
	}
	want := views.StatusMsg{Level: status.Warn, Text: metricsDown}
	if got := tuitest.All[views.StatusMsg](out); len(got) != 1 || got[0] != want {
		t.Errorf("statuses = %v, want only %+v", got, want)
	}

	tick, _ = clk.Fire()
	m, out = tuitest.Drive(t, m, tick)
	if got := tuitest.All[views.StatusMsg](out); len(got) != 0 {
		t.Errorf("the same failure on the next tick was said again: %v", got)
	}

	o.Up()
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if screen := tuitest.Screen(m); strings.Contains(screen, "stale:") || !strings.Contains(screen, tuitest.PodMulti) {
		t.Errorf("recovery left the banner or lost the rows:\n%s", screen)
	}
}

// TestStaleListKeepsItsHeight: the banner takes a line off the table.
func TestStaleListKeepsItsHeight(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m, clk := clocked(t, tuitest.NewFakeClient(t, o.Metrics()), tuitest.NamespaceDemo)
	fresh := len(strings.Split(m.View(), "\n"))
	o.Down()
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	if got := len(strings.Split(m.View(), "\n")); got > fresh {
		t.Errorf("stale view is %d lines, fresh one %d", got, fresh)
	}
}

// TestSwitchingModeDropsTheStaleRows: the nodes the new mode asks for are
// not the pods the banner sat over, so a failed first list shows the error.
func TestSwitchingModeDropsTheStaleRows(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m, clk := clocked(t, tuitest.NewFakeClient(t, o.Metrics()), tuitest.NamespaceDemo)
	o.Down()
	tick, _ := clk.Fire()
	m, _ = tuitest.Drive(t, m, tick)
	m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "metrics unavailable") || strings.Contains(screen, "stale:") || strings.Contains(screen, tuitest.PodMulti) {
		t.Errorf("nodes mode after a failed first list:\n%s", screen)
	}
}

// TestRefreshWhileFetchingSaysSo: r with a fetch in flight answers
// instead of staying silent, and starts no second fetch.
func TestRefreshWhileFetchingSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := fresh(tuitest.NewFakeClient(t), tuitest.NamespaceDemo) // the first fetch is in flight
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	want := views.StatusMsg{Level: status.Info, Text: "refresh already running"}
	if len(out) != 1 || out[0] != want {
		t.Errorf("r while fetching emitted:\n%s\nwant only %+v", tuitest.Describe(out), want)
	}
}
