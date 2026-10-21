package async_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

func TestRunReplies(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")

	tests := []struct {
		name  string
		value int
		err   error
	}{
		{name: "value", value: 42},
		{name: "error", err: boom},
		{name: "value and error", value: 7, err: boom},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tok := async.New()
			msg := async.Run(tok, time.Second, func(context.Context) (int, error) {
				return tc.value, tc.err
			})()

			res, ok := msg.(async.Result[int])
			if !ok {
				t.Fatalf("Run replied %T, want async.Result[int]", msg)
			}
			if res.Token != tok || res.Value != tc.value || !errors.Is(res.Err, tc.err) {
				t.Fatalf("Result = %+v, want {%v %d %v}", res, tok, tc.value, tc.err)
			}
		})
	}
}

func TestRunBoundsTheCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		timeout time.Duration
		bound   time.Duration
	}{
		{name: "given timeout", timeout: 3 * time.Second, bound: 3 * time.Second},
		{name: "zero takes the default", timeout: 0, bound: async.DefaultTimeout},
		{name: "negative takes the default", timeout: -time.Second, bound: async.DefaultTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := time.Now()
			msg := async.Run(async.New(), tc.timeout, func(ctx context.Context) (time.Time, error) {
				deadline, ok := ctx.Deadline()
				if !ok {
					return time.Time{}, errors.New("no deadline")
				}
				return deadline, nil
			})()
			after := time.Now()

			res := msg.(async.Result[time.Time])
			if res.Err != nil {
				t.Fatal(res.Err)
			}
			if res.Value.Before(before.Add(tc.bound)) || res.Value.After(after.Add(tc.bound)) {
				t.Fatalf("deadline %v is not %v after the call", res.Value, tc.bound)
			}
		})
	}
}

// A call that honours its context reports the deadline in its own words,
// which name what timed out.
func TestRunKeepsTheCallsDeadlineError(t *testing.T) {
	t.Parallel()
	tok := async.New()
	msg := async.Run(tok, time.Millisecond, func(ctx context.Context) (int, error) {
		<-ctx.Done()
		return 0, fmt.Errorf("list pods: %w", ctx.Err())
	})()

	res := msg.(async.Result[int])
	if res.Token != tok || !errors.Is(res.Err, context.DeadlineExceeded) || res.Err.Error() != "list pods: context deadline exceeded" {
		t.Fatalf("Result = %+v, want the token and the call's own deadline error", res)
	}
}

// A call that ignores its context (a hung exec credential plugin) must not
// hold the reply, or a poller would stay busy for good.
func TestRunRepliesAtTheDeadlineWhenTheCallHangs(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	returned := make(chan struct{})
	tok := async.New()
	cmd := async.Run(tok, 5*time.Millisecond, func(context.Context) (int, error) {
		defer close(returned)
		<-release
		return 1, nil
	})

	res := cmd().(async.Result[int])
	if res.Token != tok || res.Value != 0 || !errors.Is(res.Err, context.DeadlineExceeded) {
		t.Fatalf("Result = %+v, want the token, a zero value and context.DeadlineExceeded", res)
	}
	close(release)
	<-returned
}

func TestRunRaisesAPanicOnTheCmdGoroutine(t *testing.T) {
	t.Parallel()
	cmd := async.Run(async.New(), time.Second, func(context.Context) (int, error) {
		panic("boom")
	})

	defer func() {
		r := recover()
		text, ok := r.(string)
		if !ok || !strings.HasPrefix(text, "boom") || !strings.Contains(text, "goroutine") {
			t.Fatalf("recovered %v, want the panic value followed by the call's stack", r)
		}
	}()
	cmd()
	t.Fatal("the Cmd returned instead of panicking")
}
