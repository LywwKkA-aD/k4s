package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestRenderClockMedian(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		times []time.Duration
		slow  bool
	}{
		{name: "too few renders to judge", times: repeat(15, 9*time.Millisecond)},
		{name: "fast", times: repeat(16, time.Millisecond)},
		{name: "slow", times: repeat(16, 5*time.Millisecond), slow: true},
		{name: "a few slow ones", times: append(repeat(10, time.Millisecond), repeat(6, 20*time.Millisecond)...)},
		{name: "only the last 16 count", times: append(repeat(20, 9*time.Millisecond), repeat(16, time.Millisecond)...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := &renderClock{}
			for _, d := range tt.times {
				c.record(d)
			}
			if got := c.slow(); got != tt.slow {
				t.Fatalf("slow = %v, want %v", got, tt.slow)
			}
		})
	}
	var none *renderClock
	none.record(time.Second)
	if none.slow() {
		t.Fatal("a nil clock reported slow renders")
	}
}

func repeat(n int, d time.Duration) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		out[i] = d
	}
	return out
}

// TestSlowRendersStopMotion: once the median render runs past 4 ms the
// next frame stops the motion, and it stays off for the session: the
// memory carries it to the next dashboard.
func TestSlowRendersStopMotion(t *testing.T) {
	t.Parallel()

	frames := asynctest.NewClock(time.Date(2026, 10, 21, 14, 2, 0, 0, time.UTC))
	fetch := func(_ context.Context, _ dash.Plan, prev dash.Snapshot) dash.Snapshot { return prev }
	c := tuitest.NewFakeClient(t)
	m := New(c, nil, WithFetch(fetch), WithFrames(frames))
	m.mo = m.mo.springUp()
	m, cmd := m.animate()
	if cmd == nil {
		t.Fatal("a spring armed no frame")
	}
	for range renderSamples {
		m.perf.record(5 * time.Millisecond)
	}
	next, _ := m.onFrame(animFrameMsg{tok: m.mo.armed})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("onFrame returned %T", next)
	}
	if got.mo.moving() || !got.mem.slowView || !got.still() {
		t.Fatalf("after slow renders: moving %v, slowView %v", got.mo.moving(), got.mem.slowView)
	}
	if got.mo.spring.Pos != 1 {
		t.Fatalf("the spring stopped at %v, want its end", got.mo.spring.Pos)
	}
	again := New(c, nil, WithMemory(got.Memory()), WithFetch(fetch))
	if !again.still() {
		t.Fatal("the next dashboard of the session moves again")
	}
}
