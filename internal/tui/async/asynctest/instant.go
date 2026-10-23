package asynctest

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Instant is an async.Clock whose ticks fire as soon as their Cmd runs,
// stamped with the wall time then. A test driver feeds a motion clock's
// frames through it without waiting each one out; the frames arrive in
// the order a real clock would deliver them. A Poller must never run on
// it: its next tick would come due at once, forever.
type Instant struct{}

// Tick returns a Cmd that returns fn's message at once, whatever d is.
func (Instant) Tick(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg { return fn(time.Now()) }
}
