package tuitest

import (
	"fmt"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	// DefaultCmdTimeout is how long the driver waits for one Cmd. Watch
	// tickers (5s), cursor blinks (530ms) and stream reads that have
	// nothing to deliver never finish inside it and are skipped, so a test
	// never waits on wall-clock timers. Real work against the fake
	// clientset finishes in well under a millisecond.
	DefaultCmdTimeout = 250 * time.Millisecond

	// DefaultMaxMsgs caps how many messages one input may cause to be fed
	// back, so a Cmd that keeps re-arming itself fails the test instead of
	// hanging it.
	DefaultMaxMsgs = 1000
)

// Driver feeds messages through a model the way tea.Program does: every
// returned Cmd runs, tea.Batch children run concurrently, tea.Sequence
// children run in order, and each produced message goes back into the model.
// The zero value uses DefaultCmdTimeout and DefaultMaxMsgs.
type Driver struct {
	// CmdTimeout bounds each Cmd. A Cmd still running at the deadline is
	// abandoned: its goroutine finishes on its own and its message is
	// dropped.
	CmdTimeout time.Duration
	// MaxMsgs bounds the messages fed back for one input.
	MaxMsgs int
}

func (d Driver) cmdTimeout() time.Duration {
	if d.CmdTimeout > 0 {
		return d.CmdTimeout
	}
	return DefaultCmdTimeout
}

func (d Driver) maxMsgs() int {
	if d.MaxMsgs > 0 {
		return d.MaxMsgs
	}
	return DefaultMaxMsgs
}

// Drive feeds msgs into m one at a time with the default Driver. Each input
// settles (its Cmds run and their messages are fed back, recursively)
// before the next one goes in, like a user typing slowly. It returns the
// final model and every message the Cmds produced, in delivery order. A
// tea.QuitMsg is recorded and stops the drive, as it stops a program.
func Drive[M tea.Model](t testing.TB, m M, msgs ...tea.Msg) (M, []tea.Msg) {
	t.Helper()
	return DriveWith(t, Driver{}, m, msgs...)
}

// DriveWith is Drive with an explicit Driver.
func DriveWith[M tea.Model](t testing.TB, d Driver, m M, msgs ...tea.Msg) (M, []tea.Msg) {
	t.Helper()
	r := runner{t: t, d: d}
	var emitted []tea.Msg
	for _, in := range msgs {
		next, out, quit := settle(r, m, []tea.Msg{in})
		m = next
		emitted = append(emitted, out...)
		if quit {
			break
		}
	}
	return m, emitted
}

// Init runs m.Init() and drives the messages it produces, the way a program
// starts a model. It returns the settled model and every produced message.
func Init[M tea.Model](t testing.TB, m M) (M, []tea.Msg) {
	t.Helper()
	return InitWith(t, Driver{}, m)
}

// InitWith is Init with an explicit Driver.
func InitWith[M tea.Model](t testing.TB, d Driver, m M) (M, []tea.Msg) {
	t.Helper()
	r := runner{t: t, d: d}
	first := r.collect(m.Init())
	next, out, _ := settle(r, m, first)
	return next, slices.Concat(first, out)
}

// Step delivers one message and runs the returned Cmd, but does not feed the
// produced messages back. Use it to look at what a single Update asked for,
// for example a fetch that must not be issued twice while one is in flight.
func Step[M tea.Model](t testing.TB, m M, msg tea.Msg) (M, []tea.Msg) {
	t.Helper()
	r := runner{t: t}
	next := update(t, m, msg)
	return next.model, r.collect(next.cmd)
}

// Collect runs cmd with the default Driver and returns the messages it
// produces, expanding batches and sequences. Nothing is fed back.
func Collect(t testing.TB, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	return runner{t: t}.collect(cmd)
}

// settle drains queue through m until no message is left. The bool reports
// that a tea.QuitMsg was reached.
func settle[M tea.Model](r runner, m M, queue []tea.Msg) (M, []tea.Msg, bool) {
	r.t.Helper()
	var emitted []tea.Msg
	for fed := 0; len(queue) > 0; fed++ {
		msg := queue[0]
		queue = queue[1:]
		if _, quit := msg.(tea.QuitMsg); quit {
			return m, emitted, true
		}
		if fed == r.d.maxMsgs() {
			r.t.Fatalf("tuitest: model did not settle after %d messages, last %T", fed, msg)
		}
		next := update(r.t, m, msg)
		m = next.model
		out := r.collect(next.cmd)
		emitted = append(emitted, out...)
		queue = append(queue, out...)
	}
	return m, emitted, false
}

type updated[M tea.Model] struct {
	model M
	cmd   tea.Cmd
}

// update calls m.Update and asserts the result keeps the caller's type, so
// tests get their concrete model (or interface) back without assertions.
func update[M tea.Model](t testing.TB, m M, msg tea.Msg) updated[M] {
	t.Helper()
	next, cmd := m.Update(msg)
	typed, ok := next.(M)
	if !ok {
		t.Fatalf("tuitest: Update(%T) returned %T, want %T", msg, next, m)
	}
	return updated[M]{model: typed, cmd: cmd}
}

// runner executes Cmd trees under one Driver's limits.
type runner struct {
	t testing.TB
	d Driver
}

// collect runs cmd and returns the messages it produced: none for a nil
// Cmd, a nil message or a Cmd that missed its deadline; the expanded
// children for a batch or sequence.
func (r runner) collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg, ok := r.call(cmd)
	if !ok || msg == nil {
		return nil
	}
	if batch, isBatch := msg.(tea.BatchMsg); isBatch {
		return r.concurrently(batch)
	}
	if seq, isSeq := sequence(msg); isSeq {
		return r.inOrder(seq)
	}
	return []tea.Msg{msg}
}

func (r runner) concurrently(cmds []tea.Cmd) []tea.Msg {
	results := make([][]tea.Msg, len(cmds))
	var wg sync.WaitGroup
	for i, c := range cmds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = r.collect(c)
		}()
	}
	wg.Wait()
	return slices.Concat(results...)
}

func (r runner) inOrder(cmds []tea.Cmd) []tea.Msg {
	results := make([][]tea.Msg, 0, len(cmds))
	for _, c := range cmds {
		results = append(results, r.collect(c))
	}
	return slices.Concat(results...)
}

type callResult struct {
	msg      tea.Msg
	panicked any
}

// call runs cmd on its own goroutine and waits up to the Cmd deadline. A
// panic inside the Cmd fails the test instead of killing the test binary.
func (r runner) call(cmd tea.Cmd) (tea.Msg, bool) {
	done := make(chan callResult, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- callResult{panicked: p}
			}
		}()
		done <- callResult{msg: cmd()}
	}()
	timer := time.NewTimer(r.d.cmdTimeout())
	defer timer.Stop()
	select {
	case res := <-done:
		if res.panicked != nil {
			r.t.Errorf("tuitest: Cmd panicked: %v", res.panicked)
			return nil, false
		}
		return res.msg, true
	case <-timer.C:
		return nil, false
	}
}

var cmdType = reflect.TypeFor[tea.Cmd]()

// sequence unpacks the framework's unexported tea.Sequence message: any
// slice of Cmds that is not a tea.BatchMsg.
func sequence(msg tea.Msg) ([]tea.Cmd, bool) {
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != cmdType {
		return nil, false
	}
	out := make([]tea.Cmd, v.Len())
	for i := range out {
		c, _ := v.Index(i).Interface().(tea.Cmd)
		out[i] = c
	}
	return out, true
}

// IsExec reports whether msg asks the program to hand the terminal to an
// external process (tea.ExecProcess). The driver never runs that process.
func IsExec(msg tea.Msg) bool {
	return fmt.Sprintf("%T", msg) == "tea.execMsg"
}

// IsQuit reports whether msg is the framework's quit request.
func IsQuit(msg tea.Msg) bool {
	_, ok := msg.(tea.QuitMsg)
	return ok
}
