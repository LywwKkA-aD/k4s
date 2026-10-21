package async_test

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
)

var epoch = time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)

// newPoller returns an enabled poller on a manual clock.
func newPoller(cfg async.Config) (async.Poller, *asynctest.Clock) {
	clk := asynctest.NewClock(epoch)
	cfg.Clock = clk
	cfg.Enabled = true
	return async.NewPoller(async.New(), cfg), clk
}

// run executes cmd the way Bubble Tea would. Tick Cmds of the manual clock
// only register their timer, so any message is a mistake.
func run(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("Cmd produced %T, want only a timer", msg)
	}
}

// fire delivers the earliest pending timer as a tick.
func fire(t *testing.T, clk *asynctest.Clock) async.TickMsg {
	t.Helper()
	msg, ok := clk.Fire()
	if !ok {
		t.Fatal("no timer is pending")
	}
	tick, ok := msg.(async.TickMsg)
	if !ok {
		t.Fatalf("timer delivered %T, want async.TickMsg", msg)
	}
	return tick
}

// poll answers the poll in flight with err and runs the Cmd that follows.
func poll(t *testing.T, p async.Poller, err error) async.Poller {
	t.Helper()
	p, cmd := p.Done(p.Token(), err)
	run(t, cmd)
	return p
}

// cycle answers the poll in flight with err, then lets the armed tick
// start the next one.
func cycle(t *testing.T, p async.Poller, clk *asynctest.Clock, err error) async.Poller {
	t.Helper()
	p = poll(t, p, err)
	p, due := p.Due(fire(t, clk))
	if !due {
		t.Fatal("the armed tick did not start a poll")
	}
	return p
}

func TestNewPollerDefaults(t *testing.T) {
	t.Parallel()
	tok := async.New()
	p := async.NewPoller(tok, async.Config{})

	if p.Token() != tok || p.Interval() != async.DefaultInterval || p.Timeout() != async.DefaultTimeout {
		t.Fatalf("got token %v, interval %v, timeout %v", p.Token(), p.Interval(), p.Timeout())
	}
	if p.Enabled() || !p.Active() || p.Busy() || p.Armed() || p.BackedOff() {
		t.Fatalf("got enabled=%v active=%v busy=%v armed=%v backedOff=%v, want a disabled, active, idle poller",
			p.Enabled(), p.Active(), p.Busy(), p.Armed(), p.BackedOff())
	}
	if p.Delay() != async.DefaultInterval || p.Hint() != "paused" || p.Status() != "" {
		t.Fatalf("got delay %v, hint %q, status %q", p.Delay(), p.Hint(), p.Status())
	}

	custom := async.NewPoller(tok, async.Config{Interval: 10 * time.Second, Timeout: 15 * time.Second, Enabled: true})
	if custom.Interval() != 10*time.Second || custom.Timeout() != 15*time.Second || !custom.Enabled() {
		t.Fatalf("got interval %v, timeout %v, enabled %v", custom.Interval(), custom.Timeout(), custom.Enabled())
	}
}

func TestZeroPollerAcceptsNothing(t *testing.T) {
	t.Parallel()
	var zero async.Poller
	other, clk := newPoller(async.Config{})
	poll(t, other, nil) // arms its tick on clk

	if zero.Accept(async.Token{}) {
		t.Fatal("the zero poller accepted the zero token")
	}
	if _, due := zero.Due(fire(t, clk)); due {
		t.Fatal("the zero poller took another poller's tick")
	}
	if zero.Interval() != async.DefaultInterval || zero.Timeout() != async.DefaultTimeout {
		t.Fatal("the zero poller has no defaults")
	}
}

func TestPollerArmsAfterTheReply(t *testing.T) {
	t.Parallel()
	p, clk := newPoller(async.Config{})

	p, ok := p.Begin()
	if !ok || !p.Busy() || p.Armed() {
		t.Fatal("Begin did not start a poll")
	}
	if again, ok := p.Begin(); ok || !again.Busy() {
		t.Fatal("a second Begin started an overlapping poll")
	}

	p, cmd := p.Done(p.Token(), nil)
	if cmd == nil || p.Busy() || !p.Armed() {
		t.Fatal("Done did not arm the next tick")
	}
	run(t, cmd)
	if got, want := clk.Delays(), []time.Duration{5 * time.Second}; !slices.Equal(got, want) {
		t.Fatalf("armed %v, want %v", got, want)
	}

	tick := fire(t, clk)
	if !tick.At().Equal(epoch.Add(5 * time.Second)) {
		t.Fatalf("tick At = %v, want epoch+5s", tick.At())
	}
	p, due := p.Due(tick)
	if !due || !p.Busy() || p.Armed() {
		t.Fatal("the live tick did not start a poll")
	}
}

// The first load of a view with watch off still runs; no tick follows it.
func TestDisabledPollerPollsOnDemandOnly(t *testing.T) {
	t.Parallel()
	clk := asynctest.NewClock(epoch)
	p := async.NewPoller(async.New(), async.Config{Clock: clk})

	p, ok := p.Begin()
	if !ok {
		t.Fatal("Begin refused on a disabled poller")
	}
	p, cmd := p.Done(p.Token(), nil)
	if cmd != nil || p.Armed() || p.Busy() {
		t.Fatal("a disabled poller armed a tick")
	}
}

func TestPollerDropsStaleTicks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		stale func(t *testing.T) (async.Poller, async.TickMsg)
	}{
		{name: "another view's tick", stale: func(t *testing.T) (async.Poller, async.TickMsg) {
			mine, _ := newPoller(async.Config{})
			theirs, clk := newPoller(async.Config{})
			mine = poll(t, mine, nil)
			poll(t, theirs, nil) // arms its tick on clk
			return mine, fire(t, clk)
		}},
		{name: "a tick delivered twice", stale: func(t *testing.T) (async.Poller, async.TickMsg) {
			p, clk := newPoller(async.Config{})
			p = poll(t, p, nil)
			tick := fire(t, clk)
			p, _ = p.Due(tick)
			p = poll(t, p, nil)
			return p, tick
		}},
		{name: "a tick from before watch was toggled", stale: func(t *testing.T) (async.Poller, async.TickMsg) {
			p, clk := newPoller(async.Config{})
			p = poll(t, p, nil)
			p, _ = p.WithEnabled(false)
			p, cmd := p.WithEnabled(true)
			run(t, cmd)
			return p, fire(t, clk)
		}},
		{name: "a tick from before a manual refresh", stale: func(t *testing.T) (async.Poller, async.TickMsg) {
			p, clk := newPoller(async.Config{})
			p = poll(t, p, nil)
			p, _ = p.Begin()
			p = poll(t, p, nil)
			return p, fire(t, clk)
		}},
		{name: "a tick from before Restart", stale: func(t *testing.T) (async.Poller, async.TickMsg) {
			p, clk := newPoller(async.Config{})
			p = poll(t, p, nil)
			p = p.Restart(p.Token().Next())
			p = poll(t, p, nil)
			return p, fire(t, clk)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, tick := tc.stale(t)
			next, due := p.Due(tick)
			if due || next.Busy() != p.Busy() || next.Armed() != p.Armed() {
				t.Fatalf("a stale tick changed the poller: due=%v busy=%v armed=%v", due, next.Busy(), next.Armed())
			}
		})
	}
}

func TestPollerDropsForeignReplies(t *testing.T) {
	t.Parallel()
	p, _ := newPoller(async.Config{})
	p, _ = p.Begin()
	foreign := async.New()

	if p.Accept(foreign) || !p.Accept(p.Token()) {
		t.Fatal("Accept does not fence by token")
	}
	next, cmd := p.Done(foreign, errTimeout)
	if cmd != nil || !next.Busy() || next.BackedOff() {
		t.Fatal("a foreign reply changed the poller")
	}
}

func TestPollerRestart(t *testing.T) {
	t.Parallel()
	p, clk := newPoller(async.Config{})
	p, _ = p.Begin()
	p = poll(t, p, errTimeout)
	old := p.Token()

	p = p.Restart(old.Next())
	if p.Token() != old.Next() || !p.Busy() || p.Armed() {
		t.Fatal("Restart did not start a poll under the new token")
	}
	if p.Delay() != 10*time.Second {
		t.Fatalf("Restart dropped the backoff: delay %v, want 10s", p.Delay())
	}
	if next, cmd := p.Done(old, nil); cmd != nil || !next.Busy() {
		t.Fatal("the reply from before Restart was taken")
	}
	p = poll(t, p, nil)
	if !p.Armed() || clk.Pending() != 2 {
		t.Fatal("the reply under the new token did not arm")
	}
}

func TestWithEnabled(t *testing.T) {
	t.Parallel()
	p, clk := newPoller(async.Config{})
	p = poll(t, p, nil)
	armed := fire(t, clk)

	p, cmd := p.WithEnabled(false)
	if cmd != nil || p.Enabled() || p.Armed() || p.Hint() != "paused" {
		t.Fatal("disabling left a tick armed")
	}
	if _, due := p.Due(armed); due {
		t.Fatal("a disabled poller took its old tick")
	}
	if same, cmd := p.WithEnabled(false); cmd != nil || same.Armed() {
		t.Fatal("disabling twice armed a tick")
	}

	p, cmd = p.WithEnabled(true)
	if cmd == nil || !p.Armed() {
		t.Fatal("enabling an idle poller did not arm")
	}
	run(t, cmd)
	if _, cmd := p.WithEnabled(true); cmd != nil {
		t.Fatal("enabling an armed poller armed a second tick")
	}

	busy, _ := p.Begin()
	busy, _ = busy.WithEnabled(false)
	busy, cmd = busy.WithEnabled(true)
	if cmd != nil || busy.Armed() {
		t.Fatal("enabling a busy poller armed a tick before the reply")
	}
}

func TestWithActive(t *testing.T) {
	t.Parallel()
	p, clk := newPoller(async.Config{})
	p, _ = p.Begin()

	p, cmd := p.WithActive(false)
	if cmd != nil || p.Active() || p.Hint() != "paused" {
		t.Fatal("deactivating armed a tick")
	}
	p, cmd = p.Done(p.Token(), nil)
	if cmd != nil || p.Armed() {
		t.Fatal("an inactive poller armed after its reply")
	}

	p, cmd = p.WithActive(true)
	if cmd == nil || !p.Armed() {
		t.Fatal("reactivating did not arm")
	}
	run(t, cmd)
	tick := fire(t, clk)
	p, _ = p.WithActive(false)
	if _, due := p.Due(tick); due {
		t.Fatal("an inactive poller took its old tick")
	}
	if _, cmd := p.WithActive(false); cmd != nil {
		t.Fatal("deactivating twice armed a tick")
	}
}

func TestWithInterval(t *testing.T) {
	t.Parallel()
	p, clk := newPoller(async.Config{})
	p = poll(t, p, nil)

	p, cmd := p.WithInterval(15 * time.Second)
	if cmd != nil || p.Interval() != 15*time.Second || !p.Armed() {
		t.Fatal("a longer interval re-armed the pending tick")
	}
	p, due := p.Due(fire(t, clk))
	if !due {
		t.Fatal("the tick armed before the change was dropped")
	}
	p = poll(t, p, nil)

	p, cmd = p.WithInterval(5 * time.Second)
	if cmd == nil || !p.Armed() {
		t.Fatal("a shorter interval did not re-arm")
	}
	run(t, cmd)
	if got, want := clk.Delays(), []time.Duration{5 * time.Second, 15 * time.Second, 5 * time.Second}; !slices.Equal(got, want) {
		t.Fatalf("armed %v, want %v", got, want)
	}
	if _, due := p.Due(fire(t, clk)); !due {
		t.Fatal("the re-armed tick was dropped")
	}
	if _, due := p.Due(fire(t, clk)); due {
		t.Fatal("the replaced 15s tick was taken")
	}

	if same, cmd := p.WithInterval(5 * time.Second); cmd != nil || !same.Armed() {
		t.Fatal("setting the same interval re-armed")
	}
	if dflt, _ := p.WithInterval(0); dflt.Interval() != async.DefaultInterval {
		t.Fatalf("WithInterval(0) = %v, want the default", dflt.Interval())
	}
	idle, _ := p.WithEnabled(false)
	if _, cmd := idle.WithInterval(time.Second); cmd != nil {
		t.Fatal("an interval change armed a disabled poller")
	}
}

func TestRealClockIsTheDefault(t *testing.T) {
	t.Parallel()
	p := async.NewPoller(async.New(), async.Config{Enabled: true})
	p, cmd := p.Done(p.Token(), nil)
	if cmd == nil || !p.Armed() {
		t.Fatal("a poller without a clock did not arm")
	}

	want := struct{}{}
	msg := async.RealClock{}.Tick(time.Millisecond, func(time.Time) tea.Msg { return want })()
	if msg != want {
		t.Fatalf("RealClock tick = %v, want %v", msg, want)
	}
}
