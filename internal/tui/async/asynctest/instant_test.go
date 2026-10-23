package asynctest_test

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
)

var _ async.Clock = asynctest.Instant{}

// TestInstantFiresAtOnce: a tick of any delay delivers its message as soon
// as its Cmd runs, stamped with the wall time then, so a driver feeds a
// motion clock's frames without waiting them out.
func TestInstantFiresAtOnce(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		delay time.Duration
	}{
		{name: "a frame", delay: 50 * time.Millisecond},
		{name: "longer than any driver waits", delay: time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := time.Now()
			began := time.Now()
			msg := asynctest.Instant{}.Tick(tc.delay, named(tc.name))()
			if took := time.Since(began); took >= tc.delay {
				t.Errorf("the tick took %v, want less than its %v delay", took, tc.delay)
			}
			got, ok := msg.(fired)
			if !ok || got.name != tc.name {
				t.Fatalf("tick = %#v, want fired %q", msg, tc.name)
			}
			if got.at.Before(before) || got.at.After(time.Now()) {
				t.Errorf("tick stamped %v, want the wall time it ran", got.at)
			}
		})
	}
}
