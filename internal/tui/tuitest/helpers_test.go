package tuitest_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// keyMsg returns msg as the fmt.Stringer key.Matches wants.
func keyMsg(t *testing.T, msg any) fmt.Stringer {
	t.Helper()
	s, ok := msg.(fmt.Stringer)
	if !ok {
		t.Fatalf("%T is not a key message", msg)
	}
	return s
}

// recorder is a testing.TB that records failures instead of failing the
// real test, so the harness's own failure paths can be asserted.
type recorder struct {
	testing.TB
	mu     sync.Mutex
	fatals []string
	errs   []string
}

func (r *recorder) Helper() {}

func (r *recorder) Fatalf(format string, args ...any) {
	r.mu.Lock()
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
	r.mu.Unlock()
	runtime.Goexit()
}

func (r *recorder) Errorf(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
}

func (r *recorder) failures() (fatals, errs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.fatals...), append([]string(nil), r.errs...)
}

// record runs fn against a recorder on its own goroutine, so a Fatalf that
// calls runtime.Goexit ends fn and not the calling test.
func record(fn func(tb testing.TB)) *recorder {
	r := &recorder{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(r)
	}()
	<-done
	return r
}
