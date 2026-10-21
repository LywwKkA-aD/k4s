package async

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// DefaultInterval is the base cadence of a Poller configured without one.
const DefaultInterval = 5 * time.Second

// Config configures a Poller. Zero fields take the defaults.
type Config struct {
	// Interval is the base cadence; zero means DefaultInterval.
	Interval time.Duration
	// Timeout bounds each poll the owner runs; zero means DefaultTimeout.
	Timeout time.Duration
	// Enabled is the owner's switch: watch on and a client to poll with.
	// A disabled poller never arms a tick; Begin still runs a poll on
	// demand (the first load, 'r').
	Enabled bool
	// Clock arms the ticks; nil means RealClock.
	Clock Clock
	// Classify maps a poll's error to an Outcome; nil means Classify.
	Classify Classifier
}

// Poller owns a view's refresh cadence: the tick, the busy flag, the
// generation that retires replaced ticks, the per-poll timeout and the
// backoff. It is a value; every change returns the next Poller.
//
// The next tick is armed when a poll is answered (Done), not when the
// previous tick fires, so polls never overlap and a slow answer pushes the
// next one back. A poller arms only while it is enabled, active and idle,
// and disabling or deactivating it retires the armed tick: a view with
// watch off, without a client, or off screen schedules nothing.
//
// A Cmd a method returns must reach Bubble Tea. A dropped one leaves the
// poller waiting on a tick that never comes, until the next Begin.
type Poller struct {
	tok      Token
	clock    Clock
	classify Classifier
	interval time.Duration
	timeout  time.Duration

	enabled bool
	active  bool
	busy    bool
	armed   bool
	// gen numbers arms; only a tick of the latest one is live.
	gen      uint64
	armedFor time.Duration

	backoff backoff
}

// NewPoller returns an idle, active poller whose polls and ticks carry
// tok, a Token from New.
func NewPoller(tok Token, cfg Config) Poller {
	return Poller{
		tok:      tok,
		clock:    cfg.Clock,
		classify: cfg.Classify,
		interval: cfg.Interval,
		timeout:  cfg.Timeout,
		enabled:  cfg.Enabled,
		active:   true,
	}
}

// Token is the Token the owner runs each poll under.
func (p Poller) Token() Token { return p.tok }

// Timeout is the deadline the owner gives each poll.
func (p Poller) Timeout() time.Duration { return orDefault(p.timeout, DefaultTimeout) }

// Interval is the base cadence, before any backoff.
func (p Poller) Interval() time.Duration { return orDefault(p.interval, DefaultInterval) }

// Enabled reports the owner's switch.
func (p Poller) Enabled() bool { return p.enabled }

// Active reports whether the owning view is on screen.
func (p Poller) Active() bool { return p.active }

// Busy reports whether a poll is in flight.
func (p Poller) Busy() bool { return p.busy }

// Armed reports whether a tick is scheduled.
func (p Poller) Armed() bool { return p.armed }

// Accept reports whether a reply carrying tok answers this poller's
// current poll. Replies for another view, or from before Restart, do not.
func (p Poller) Accept(tok Token) bool { return p.tok.Accept(tok) }

// Delay is the wait before the next tick: the interval after backoff and
// any Retry-After.
func (p Poller) Delay() time.Duration { return p.backoff.delay(p.Interval()) }

// BackedOff reports whether the API's answers have stretched the delay
// past the interval.
func (p Poller) BackedOff() bool { return p.Delay() > p.Interval() }

// Status is the footer note while backed off, "refresh 20s (slow API)" or
// "refresh 20s (throttled)", and empty otherwise or while paused.
func (p Poller) Status() string {
	if !p.enabled || !p.active || !p.BackedOff() {
		return ""
	}
	return "refresh " + seconds(p.Delay()) + " (" + p.backoff.reason() + ")"
}

// Hint is the cadence for the key hints: "watch 5s", "watch 20s (slow
// API)" while backed off, and "paused" while disabled or inactive.
func (p Poller) Hint() string {
	if !p.enabled || !p.active {
		return "paused"
	}
	if !p.BackedOff() {
		return "watch " + seconds(p.Delay())
	}
	return "watch " + seconds(p.Delay()) + " (" + p.backoff.reason() + ")"
}

// Begin starts a poll on demand: the first load or 'r'. It works whether
// or not the poller is enabled and retires the armed tick; Done arms the
// next. ok is false while a poll is in flight, and the caller sends
// nothing.
func (p Poller) Begin() (Poller, bool) {
	if p.busy {
		return p, false
	}
	p.armed = false
	p.busy = true
	return p, true
}

// Due takes a tick. ok reports that the tick is this poller's live one and
// a poll has started, which the caller now sends; any other tick is
// dropped and the poller is returned as it was.
func (p Poller) Due(msg TickMsg) (Poller, bool) {
	if !p.armed || msg.gen != p.gen || !p.tok.Accept(msg.tok) {
		return p, false
	}
	p.armed = false
	p.busy = true
	return p, true
}

// Done folds in the answer to the current poll, carried under tok, and
// arms the next tick when the poller may. A reply for another Token
// changes nothing. err drives the backoff through the Classifier.
func (p Poller) Done(tok Token, err error) (Poller, tea.Cmd) {
	if !p.Accept(tok) {
		return p, nil
	}
	p.busy = false
	p.backoff = p.backoff.observe(p.classifier()(err))
	return p.arm()
}

// Restart moves the poller onto tok, a fresh Token from the owner's mint,
// and starts a poll under it at once: the namespace or mode the view shows
// has changed. The poll in flight and the armed tick are abandoned and
// their replies dropped; the backoff stays, since the API is the same.
func (p Poller) Restart(tok Token) Poller {
	p.tok = tok
	p.armed = false
	p.busy = true
	return p
}

// WithEnabled sets the owner's switch ('w', or a client appearing).
// Enabling an idle poller arms a tick; disabling retires it.
func (p Poller) WithEnabled(on bool) (Poller, tea.Cmd) {
	if on == p.enabled {
		return p, nil
	}
	p.enabled = on
	return p.settle()
}

// WithActive tells the poller whether its view is on screen. Going
// inactive retires the armed tick; coming back arms one when enabled.
func (p Poller) WithActive(on bool) (Poller, tea.Cmd) {
	if on == p.active {
		return p, nil
	}
	p.active = on
	return p.settle()
}

// WithInterval changes the base cadence from outside, for the dashboard's
// blur, idle and adaptive rates (zero means DefaultInterval). The new rate
// applies from the next arm; an armed tick is replaced only when the new
// delay is shorter, so calling this on every update never postpones a
// refresh.
func (p Poller) WithInterval(d time.Duration) (Poller, tea.Cmd) {
	p.interval = d
	if !p.armed || p.Delay() >= p.armedFor {
		return p, nil
	}
	p.armed = false
	return p.arm()
}

func (p Poller) settle() (Poller, tea.Cmd) {
	if !p.enabled || !p.active {
		p.armed = false
		return p, nil
	}
	return p.arm()
}

// arm schedules the next tick under a new generation, which retires every
// earlier tick of this poller.
func (p Poller) arm() (Poller, tea.Cmd) {
	if p.armed || p.busy || !p.enabled || !p.active {
		return p, nil
	}
	p.gen++
	p.armed = true
	p.armedFor = p.Delay()
	tok, gen := p.tok, p.gen
	return p, p.clockOrReal().Tick(p.armedFor, func(at time.Time) tea.Msg {
		return TickMsg{tok: tok, gen: gen, at: at}
	})
}

func (p Poller) clockOrReal() Clock {
	if p.clock == nil {
		return RealClock{}
	}
	return p.clock
}

func (p Poller) classifier() Classifier {
	if p.classify == nil {
		return Classify
	}
	return p.classify
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}
