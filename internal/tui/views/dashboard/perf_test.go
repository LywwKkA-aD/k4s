package dashboard

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestRenderClockMedian: the switch reads the median of the last 16
// renders after the warm-up: the first renders of a dashboard draw
// everything cold and do not count, and fast renders after slow ones
// clear it again.
func TestRenderClockMedian(t *testing.T) {
	t.Parallel()

	warm := func(rest ...[]time.Duration) []time.Duration {
		return slices.Concat(append([][]time.Duration{repeat(renderWarmup, 50*time.Millisecond)}, rest...)...)
	}
	tests := []struct {
		name  string
		times []time.Duration
		slow  bool
	}{
		{name: "too few renders to judge", times: warm(repeat(15, 9*time.Millisecond))},
		{name: "fast", times: warm(repeat(16, time.Millisecond))},
		{name: "slow", times: warm(repeat(16, 5*time.Millisecond)), slow: true},
		{name: "a few slow ones", times: warm(repeat(10, time.Millisecond), repeat(6, 20*time.Millisecond))},
		{name: "fast renders clear a slow run", times: warm(repeat(20, 9*time.Millisecond), repeat(16, time.Millisecond))},
		{name: "the cold first renders do not count", times: slices.Concat(repeat(renderWarmup, time.Second), repeat(16, time.Millisecond))},
		{name: "the warm-up is not part of the window", times: repeat(renderWarmup+15, 9*time.Millisecond)},
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
	if none.slow() || none.renders() != 0 {
		t.Fatal("a nil clock reported renders")
	}
}

// TestUnchangedFrameIsNotRedrawn: a frame asked for again with nothing it
// is drawn from changed comes back as it was, without a render; a key
// that moves the focus draws anew.
func TestUnchangedFrameIsNotRedrawn(t *testing.T) {
	t.Parallel()

	frames := asynctest.NewClock(time.Date(2026, 10, 21, 14, 2, 0, 0, time.UTC))
	fetch := func(_ context.Context, _ dash.Plan, prev dash.Snapshot) dash.Snapshot { return prev }
	m, _ := tuitest.Mount(t, New(tuitest.NewFakeClient(t), nil, WithFetch(fetch), WithFrames(frames)), 207, 52)
	first, drawn := m.View(), m.perf.renders()
	if again := m.View(); again != first || m.perf.renders() != drawn {
		t.Fatalf("an unchanged frame was drawn again (%d renders, then %d)", drawn, m.perf.renders())
	}
	moved, _ := tuitest.Drive(t, m, tuitest.Press("tab"))
	if moved.View() == first || m.perf.renders() != drawn+1 {
		t.Fatalf("a focus move did not draw a new frame (%d renders)", m.perf.renders())
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
// next frame stops the motion. Faster renders let it move again, and the
// next dashboard starts with its own clock: a slow run is not carried
// across visits.
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
	for range renderWarmup + renderSamples {
		m.perf.record(5 * time.Millisecond)
	}
	next, _ := m.onFrame(animFrameMsg{tok: m.mo.armed})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("onFrame returned %T", next)
	}
	if got.mo.moving() || !got.still() {
		t.Fatalf("after slow renders: moving %v, still %v", got.mo.moving(), got.still())
	}
	if got.mo.spring.Pos != 1 {
		t.Fatalf("the spring stopped at %v, want its end", got.mo.spring.Pos)
	}

	again := New(c, nil, WithMemory(got.Memory()), WithFetch(fetch))
	if again.still() {
		t.Fatal("the next dashboard inherited the slow run")
	}

	for range renderSamples {
		got.perf.record(time.Millisecond)
	}
	if got.still() {
		t.Fatal("fast renders did not clear the slow run")
	}
}
