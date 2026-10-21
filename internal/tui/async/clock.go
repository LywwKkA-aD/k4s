package async

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Clock arms the timers behind a Poller's ticks. Tick has the shape of
// tea.Tick: the returned Cmd delivers fn's message once d has passed.
// Tests pass a manual clock (asynctest.Clock) so no test waits on time.
type Clock interface {
	Tick(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd
}

// RealClock arms ticks with tea.Tick. It is the Clock of a Poller
// configured without one.
type RealClock struct{}

// Tick returns tea.Tick(d, fn).
func (RealClock) Tick(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return tea.Tick(d, fn)
}

// TickMsg is a Poller's timer going off. Every polling view receives the
// same message type, so Poller.Due takes only the tick of its own latest
// arm and drops one armed by another view, under an earlier Token, or
// before a disable, a refresh or an interval change replaced it.
type TickMsg struct {
	tok Token
	gen uint64
	at  time.Time
}

// At is the clock's time when the tick went off.
func (m TickMsg) At() time.Time {
	return m.at
}
