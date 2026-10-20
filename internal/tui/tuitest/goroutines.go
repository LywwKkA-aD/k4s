package tuitest

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	pollInterval = 10 * time.Millisecond
	// settleRounds is how many equal consecutive samples count as settled.
	settleRounds = 3
	// settleLimit bounds the wait for a settled count.
	settleLimit = 2 * time.Second
)

// Eventually polls cond until it holds or timeout passes, then fails the
// test with msg.
func Eventually(t testing.TB, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("after %v: %s", timeout, msg)
		}
		time.Sleep(pollInterval)
	}
}

// SettledGoroutines returns runtime.NumGoroutine once the count has stopped
// moving, so a baseline is not taken while earlier work is still winding
// down. Tests that use it must not run in parallel: the count is
// process-wide.
func SettledGoroutines() int {
	prev := runtime.NumGoroutine()
	same := 0
	deadline := time.Now().Add(settleLimit)
	for same < settleRounds && time.Now().Before(deadline) {
		time.Sleep(pollInterval)
		cur := runtime.NumGoroutine()
		if cur == prev {
			same++
			continue
		}
		prev, same = cur, 0
	}
	return prev
}

// WaitGoroutines polls until at most want goroutines are left or timeout
// passes. On timeout it fails the test and dumps the stacks whose frames
// mention any of the given package paths (every stack when none is given),
// which names the goroutine that leaked.
func WaitGoroutines(t testing.TB, want int, timeout time.Duration, pkgs ...string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for runtime.NumGoroutine() > want {
		if time.Now().After(deadline) {
			t.Fatalf("goroutines: %d, want at most %d after %v\n%s",
				runtime.NumGoroutine(), want, timeout, Stacks(pkgs...))
		}
		time.Sleep(pollInterval)
	}
}

// Stacks returns the stacks of every goroutine whose frames mention one of
// pkgs, or of all goroutines when pkgs is empty.
func Stacks(pkgs ...string) string {
	buf := make([]byte, 1<<20)
	buf = buf[:runtime.Stack(buf, true)]
	stacks := strings.Split(string(buf), "\n\n")
	if len(pkgs) == 0 {
		return strings.Join(stacks, "\n\n")
	}
	out := make([]string, 0, len(stacks))
	for _, s := range stacks {
		for _, p := range pkgs {
			if strings.Contains(s, p) {
				out = append(out, s)
				break
			}
		}
	}
	return strings.Join(out, "\n\n")
}
