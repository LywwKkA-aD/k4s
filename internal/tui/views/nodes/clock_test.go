package nodes_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/nodes"
)

// TestAgeIsMeasuredFromTheViewsClock: AGE counts back from the clock the
// view was given, so a golden reads the same on any day, and a tick a day
// later ages every node by a day.
func TestAgeIsMeasuredFromTheViewsClock(t *testing.T) {
	t.Parallel()

	m, clk := clocked(client(t))
	m, _ = tuitest.Mount(t, m, width, height)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "2d1h") {
		t.Fatalf("rows at the epoch:\n%s", strings.Join(got, "\n"))
	}
	m, _ = tuitest.Drive(t, m, clk.Advance(24*time.Hour)...)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "3d1h") {
		t.Errorf("rows a day later:\n%s", strings.Join(got, "\n"))
	}
}

// TestWithNowAlone: a view given only a time source measures from it.
func TestWithNowAlone(t *testing.T) {
	t.Parallel()

	later := func() time.Time { return epoch.Add(48 * time.Hour) }
	m, _ := tuitest.Mount(t, nodes.New(client(t), nodes.WithNow(later)), width, height)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "4d1h") {
		t.Errorf("rows two days later:\n%s", strings.Join(got, "\n"))
	}
}
