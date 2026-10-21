package async_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

func TestInFlightZeroValueIsEmpty(t *testing.T) {
	t.Parallel()
	var f async.InFlight
	tok := async.New()
	if f.Busy("exec") || f.Accept("exec", tok) {
		t.Fatal("the zero InFlight reports a request")
	}
	if _, ok := f.End("exec", tok); ok {
		t.Fatal("End on the zero InFlight succeeded")
	}
}

// A second 'e' while the first resolves must emit nothing: first wins.
func TestInFlightBeginFirstWins(t *testing.T) {
	t.Parallel()
	first, second := async.New(), async.New()
	var empty async.InFlight

	one, ok := empty.Begin("exec", first)
	if !ok || !one.Busy("exec") || !one.Accept("exec", first) {
		t.Fatal("Begin on an idle action did not start it")
	}
	if empty.Busy("exec") {
		t.Fatal("Begin changed its receiver")
	}

	two, ok := one.Begin("exec", second)
	if ok {
		t.Fatal("a second Begin for a busy action succeeded")
	}
	if !two.Accept("exec", first) || two.Accept("exec", second) {
		t.Fatal("a refused Begin replaced the request in flight")
	}

	both, ok := one.Begin("logs", second)
	if !ok || !both.Accept("exec", first) || !both.Accept("logs", second) {
		t.Fatal("actions are not independent")
	}
}

func TestInFlightEnd(t *testing.T) {
	t.Parallel()
	live, stale := async.New(), async.New()
	busy, _ := async.InFlight{}.Begin("exec", live)

	tests := []struct {
		name     string
		action   string
		tok      async.Token
		ended    bool
		busyThen bool
	}{
		{name: "the live reply ends it", action: "exec", tok: live, ended: true, busyThen: false},
		{name: "a stale reply leaves it", action: "exec", tok: stale, ended: false, busyThen: true},
		{name: "a zero token leaves it", action: "exec", tok: async.Token{}, ended: false, busyThen: true},
		{name: "another action leaves it", action: "logs", tok: live, ended: false, busyThen: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next, ended := busy.End(tc.action, tc.tok)
			if ended != tc.ended {
				t.Fatalf("End = %v, want %v", ended, tc.ended)
			}
			if next.Busy("exec") != tc.busyThen {
				t.Fatalf("Busy after End = %v, want %v", next.Busy("exec"), tc.busyThen)
			}
			if !busy.Busy("exec") {
				t.Fatal("End changed its receiver")
			}
		})
	}
}

// 'l' on another row wins over a resolve still running; 'l' again on the
// same row is a repeat and is dropped.
func TestInFlightSupersedeLastWins(t *testing.T) {
	t.Parallel()
	a, b, c := async.New(), async.New(), async.New()

	onAPI, ok := async.InFlight{}.Supersede("logs", "shop/api", a)
	if !ok || !onAPI.Accept("logs", a) {
		t.Fatal("Supersede on an idle action did not start it")
	}

	repeat, ok := onAPI.Supersede("logs", "shop/api", b)
	if ok || !repeat.Accept("logs", a) || repeat.Accept("logs", b) {
		t.Fatal("a repeat for the same key replaced the request in flight")
	}

	onWeb, ok := onAPI.Supersede("logs", "shop/web", c)
	if !ok || onWeb.Accept("logs", a) || !onWeb.Accept("logs", c) {
		t.Fatal("a request for another key did not replace the one in flight")
	}
	if !onAPI.Accept("logs", a) {
		t.Fatal("Supersede changed its receiver")
	}
	if _, ended := onWeb.End("logs", a); ended {
		t.Fatal("the superseded reply ended the newer request")
	}
}
