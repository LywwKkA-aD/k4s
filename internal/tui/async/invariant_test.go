package async_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// instant is a Clock whose tick Cmds fire at once, so the walk below holds
// every timer's message without waiting on any.
type instant struct{}

func (instant) Tick(_ time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg { return fn(time.Time{}) }
}

// walkState is a poller plus every tick any step armed, live or stale.
type walkState struct {
	p     async.Poller
	ticks []async.TickMsg
}

type walkOp struct {
	name  string
	apply func(t *testing.T, s walkState) walkState
}

// armed records the tick cmd arms. A Cmd may only come back from a poller
// that is enabled, active and armed.
func armed(t *testing.T, s walkState, p async.Poller, cmd tea.Cmd) walkState {
	t.Helper()
	if cmd == nil {
		return walkState{p: p, ticks: s.ticks}
	}
	if !p.Enabled() || !p.Active() || !p.Armed() {
		t.Fatalf("a Cmd came back from enabled=%v active=%v armed=%v", p.Enabled(), p.Active(), p.Armed())
	}
	tick, ok := cmd().(async.TickMsg)
	if !ok {
		t.Fatal("the Cmd is not a tick")
	}
	return walkState{p: p, ticks: slices.Concat(s.ticks, []async.TickMsg{tick})}
}

func modify(f func(async.Poller) (async.Poller, tea.Cmd)) func(*testing.T, walkState) walkState {
	return func(t *testing.T, s walkState) walkState {
		p, cmd := f(s.p)
		return armed(t, s, p, cmd)
	}
}

func reply(err error) func(*testing.T, walkState) walkState {
	return modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.Done(p.Token(), err) })
}

func deliver(pick func([]async.TickMsg) []async.TickMsg) func(*testing.T, walkState) walkState {
	return func(_ *testing.T, s walkState) walkState {
		p := s.p
		for _, tick := range pick(s.ticks) {
			p, _ = p.Due(tick)
		}
		return walkState{p: p, ticks: s.ticks}
	}
}

func walkOps() []walkOp {
	foreign := async.New()
	return []walkOp{
		{"begin", func(_ *testing.T, s walkState) walkState { p, _ := s.p.Begin(); return walkState{p: p, ticks: s.ticks} }},
		{"reply ok", reply(nil)},
		{"reply timeout", reply(errTimeout)},
		{"reply 429", reply(errThrottled)},
		{"foreign reply", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.Done(foreign, nil) })},
		{"enable", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithEnabled(true) })},
		{"disable", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithEnabled(false) })},
		{"activate", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithActive(true) })},
		{"deactivate", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithActive(false) })},
		{"shorten", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithInterval(time.Second) })},
		{"lengthen", modify(func(p async.Poller) (async.Poller, tea.Cmd) { return p.WithInterval(30 * time.Second) })},
		{"restart", func(_ *testing.T, s walkState) walkState {
			return walkState{p: s.p.Restart(s.p.Token().Next()), ticks: s.ticks}
		}},
		{"latest tick", deliver(func(ticks []async.TickMsg) []async.TickMsg {
			if len(ticks) == 0 {
				return nil
			}
			return ticks[len(ticks)-1:]
		})},
		{"every tick", deliver(func(ticks []async.TickMsg) []async.TickMsg { return ticks })},
	}
}

// checkInvariants holds after every step of every sequence:
//   - a disabled or inactive poller is not armed and takes no tick;
//   - an armed poller is not busy;
//   - exactly one tick is live while armed, none otherwise.
func checkInvariants(t *testing.T, path []string, s walkState) {
	t.Helper()
	live := 0
	for _, tick := range s.ticks {
		if _, due := s.p.Due(tick); due {
			live++
		}
	}
	p := s.p
	fail := func(what string) {
		t.Helper()
		t.Fatalf("after %s: %s (enabled=%v active=%v busy=%v armed=%v live=%d)",
			strings.Join(path, ", "), what, p.Enabled(), p.Active(), p.Busy(), p.Armed(), live)
	}
	switch {
	case (!p.Enabled() || !p.Active()) && (p.Armed() || live > 0):
		fail("armed while disabled or inactive")
	case p.Armed() && p.Busy():
		fail("armed while busy")
	case live > 1:
		fail("more than one live tick")
	case p.Armed() != (live == 1):
		fail("armed does not match the live tick")
	}
}

func walk(t *testing.T, ops []walkOp, s walkState, path []string, depth int) {
	t.Helper()
	if depth == 0 {
		return
	}
	for _, op := range ops {
		next := op.apply(t, s)
		at := slices.Concat(path, []string{op.name})
		checkInvariants(t, at, next)
		walk(t, ops, next, at, depth-1)
	}
}

// TestPollerNeverArmsWhileDisabledOrInactive is the idle guard for every
// view that polls: it walks every sequence of up to four operations from
// a disabled and an enabled start and checks the invariants at each step.
func TestPollerNeverArmsWhileDisabledOrInactive(t *testing.T) {
	t.Parallel()
	ops := walkOps()
	for _, enabled := range []bool{false, true} {
		start := walkState{p: async.NewPoller(async.New(), async.Config{Enabled: enabled, Clock: instant{}})}
		checkInvariants(t, nil, start)
		walk(t, ops, start, nil, 4)
	}
}
