package async

import (
	"context"
	"fmt"
	"runtime/debug"
	"time"

	tea "charm.land/bubbletea/v2"
)

// DefaultTimeout bounds a Run given no positive timeout, and the polls of a
// Poller configured without one.
const DefaultTimeout = 5 * time.Second

// cancelGrace is how long Run waits past the deadline for a call that
// honours its context: its own error names the verb and the URL, which the
// bare deadline does not.
const cancelGrace = 50 * time.Millisecond

// Result is the message a Run replies with: the Token the request was
// started under, and the value and error of the call.
type Result[T any] struct {
	Token Token
	Value T
	Err   error
}

// Run returns a Cmd that calls fn with a context that expires after timeout
// (DefaultTimeout when timeout is not positive) and replies with a
// Result[T] under tok.
//
// The reply comes shortly after the deadline even when fn ignores its
// context, as an exec credential plugin waiting on a browser login does:
// Value is then the zero T, Err is context.DeadlineExceeded, and fn's late
// answer is dropped. Without this a hung call would leave a poller busy for
// good. A panic in fn is raised again on the Cmd's goroutine, where Bubble
// Tea restores the terminal before the program exits.
func Run[T any](tok Token, timeout time.Duration, fn func(context.Context) (T, error)) tea.Cmd {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		out := make(chan reply[T], 1)
		go call(ctx, fn, out)
		deadline := ctx.Done()
		var grace <-chan time.Time
		for {
			select {
			case r := <-out:
				return r.result(tok)
			case <-deadline:
				deadline, grace = nil, time.After(cancelGrace)
			case <-grace:
				return Result[T]{Token: tok, Err: ctx.Err()}
			}
		}
	}
}

// reply is what call hands back: fn's answer, or the panic it raised.
type reply[T any] struct {
	value    T
	err      error
	panicked string
}

func (r reply[T]) result(tok Token) Result[T] {
	if r.panicked != "" {
		panic(r.panicked)
	}
	return Result[T]{Token: tok, Value: r.value, Err: r.err}
}

// call runs fn on its own goroutine. out is buffered, so a late answer
// never blocks it.
func call[T any](ctx context.Context, fn func(context.Context) (T, error), out chan<- reply[T]) {
	defer func() {
		if p := recover(); p != nil {
			out <- reply[T]{panicked: fmt.Sprintf("%v\n\n%s", p, debug.Stack())}
		}
	}()
	v, err := fn(ctx)
	out <- reply[T]{value: v, err: err}
}
