package overlay_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// feed sends msgs to o in order. It returns the overlay after the last one
// (nil once closed) and the messages the closing input produced: overlays
// only answer when they close, so the Cmds of earlier inputs (cursor
// blinks) are not run.
func feed(t *testing.T, o overlay.Overlay, msgs ...tea.Msg) (overlay.Overlay, []tea.Msg) {
	t.Helper()
	for _, msg := range msgs {
		if o == nil {
			t.Fatalf("%T sent after the overlay closed", msg)
		}
		var cmd tea.Cmd
		o, cmd = o.Update(msg)
		if o == nil {
			return nil, tuitest.Collect(t, cmd)
		}
	}
	return o, nil
}

// keys is tuitest.Keys.
func keys(names ...string) []tea.Msg { return tuitest.Keys(names...) }

// screen is o's view at 80 columns, ANSI stripped.
func screen(o overlay.Overlay) string {
	return ansi.Strip(o.View(80))
}

func requireOpen(t *testing.T, o overlay.Overlay) {
	t.Helper()
	if o == nil {
		t.Fatal("overlay closed")
	}
}

func requireShows(t *testing.T, o overlay.Overlay, wants ...string) {
	t.Helper()
	requireOpen(t, o)
	got := screen(o)
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Fatalf("view lacks %q:\n%s", w, got)
		}
	}
}

func requireNarrow(t *testing.T, o overlay.Overlay, width int) {
	t.Helper()
	for _, l := range strings.Split(o.View(width), "\n") {
		if w := ansi.StringWidth(l); w > width {
			t.Fatalf("line is %d wide at width %d: %q", w, width, ansi.Strip(l))
		}
	}
}

// clock is a settable time source.
type clock struct{ now time.Time }

func newClock() *clock { return &clock{now: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)} }

func (c *clock) Now() time.Time { return c.now }

func (c *clock) Advance(d time.Duration) { c.now = c.now.Add(d) }
