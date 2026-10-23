// Package asynctest provides clocks for tests: Clock, a manual one for
// views that poll through async.Poller, and Instant, whose ticks fire at
// once, for motion frames.
package asynctest

import (
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Clock is a manual async.Clock. A tick Cmd registers its timer when it
// runs and returns nil at once, so a test driver never waits on it; Fire
// and Advance then deliver the messages of the timers that come due. The
// zero value starts at the zero time. Safe for concurrent use.
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	timers []timer
	delays []time.Duration
}

type timer struct {
	due time.Time
	fn  func(time.Time) tea.Msg
}

// NewClock returns a Clock reading start.
func NewClock(start time.Time) *Clock {
	return &Clock{now: start}
}

// Tick returns a Cmd that registers a timer for d from the clock's time
// when the Cmd runs.
func (c *Clock) Tick(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.timers = append(slices.Clip(c.timers), timer{due: c.now.Add(d), fn: fn})
		c.delays = append(slices.Clip(c.delays), d)
		return nil
	}
}

// Fire moves the clock to the earliest pending timer and returns its
// message; timers due together fire in the order they were armed. ok is
// false when nothing is pending.
func (c *Clock) Fire() (tea.Msg, bool) {
	c.mu.Lock()
	if len(c.timers) == 0 {
		c.mu.Unlock()
		return nil, false
	}
	first := 0
	for i, t := range c.timers {
		if t.due.Before(c.timers[first].due) {
			first = i
		}
	}
	t := c.timers[first]
	c.timers = slices.Delete(slices.Clone(c.timers), first, first+1)
	c.now = later(c.now, t.due)
	c.mu.Unlock()
	return t.fn(t.due), true
}

// Advance moves the clock forward by d and returns the messages of every
// timer due by then, earliest first.
func (c *Clock) Advance(d time.Duration) []tea.Msg {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due, rest []timer
	for _, t := range c.timers {
		if t.due.After(c.now) {
			rest = append(rest, t)
			continue
		}
		due = append(due, t)
	}
	c.timers = rest
	c.mu.Unlock()

	slices.SortStableFunc(due, func(a, b timer) int { return a.due.Compare(b.due) })
	msgs := make([]tea.Msg, 0, len(due))
	for _, t := range due {
		msgs = append(msgs, t.fn(t.due))
	}
	return msgs
}

// Delays returns every delay a tick was registered with, in order.
func (c *Clock) Delays() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.delays)
}

// Pending is the number of registered timers that have not fired.
func (c *Clock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// Now is the clock's time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
