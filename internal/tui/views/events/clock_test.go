package events_test

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/events"
)

// TestLastSeenIsMeasuredFromTheViewsClock: LAST SEEN counts back from the
// clock the view was given, so a golden reads the same on any day, and a
// tick an hour later ages every row by an hour.
func TestLastSeenIsMeasuredFromTheViewsClock(t *testing.T) {
	t.Parallel()

	m, clk := clocked(client(t), demo)
	m, _ = tuitest.Mount(t, m, width, height)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "10m (x5 over 15h)") {
		t.Fatalf("rows at the epoch:\n%s", strings.Join(got, "\n"))
	}
	m, _ = tuitest.Drive(t, m, clk.Advance(time.Hour)...)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "70m (x5 over 16h)") {
		t.Errorf("rows an hour later:\n%s", strings.Join(got, "\n"))
	}
}

// TestWithNowAlone: a view given only a time source measures from it.
func TestWithNowAlone(t *testing.T) {
	t.Parallel()

	later := func() time.Time { return epoch.Add(24 * time.Hour) }
	m, _ := tuitest.Mount(t, events.New(client(t), demo, events.WithNow(later)), width, height)
	if got := rows(m); len(got) == 0 || !strings.Contains(got[0], "24h (x5 over 39h)") {
		t.Errorf("rows a day later:\n%s", strings.Join(got, "\n"))
	}
}
