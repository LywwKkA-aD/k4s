package viz_test

import (
	"math"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// framesToSettle steps s until it meets the stop rule at width, up to a
// limit, and returns the frame count.
func framesToSettle(s viz.Spring, width, limit int) int {
	for n := 0; n <= limit; n++ {
		if s.Settled(width) {
			return n
		}
		s = s.Step()
	}
	return limit + 1
}

// The spec budget: a meter springing up from 0 on the first snapshot is
// done within 8 frames of the 50 ms clock.
func TestSpringZeroToOneAtWidthTwelveSettlesWithinEightFrames(t *testing.T) {
	t.Parallel()
	n := framesToSettle(viz.NewSpring(0, 1), 12, 100)
	if n > viz.MaxFrames {
		t.Fatalf("settled after %d frames, want <= %d", n, viz.MaxFrames)
	}
	if n < 4 {
		t.Errorf("settled after %d frames: too fast to read as motion", n)
	}
}

func TestSpringIsCriticallyDampedFromRest(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		from, to float64
	}{
		{name: "up", from: 0, to: 1},
		{name: "down", from: 1, to: 0},
		{name: "wide range", from: -40, to: 250},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s := viz.NewSpring(c.from, c.to)
			dir := math.Copysign(1, c.to-c.from)
			prev := s.Pos
			for range 30 {
				s = s.Step()
				if (s.Pos-prev)*dir < -1e-12 {
					t.Fatalf("moved backwards: %v after %v", s.Pos, prev)
				}
				if (s.Pos-c.to)*dir > 1e-12 {
					t.Fatalf("overshot %v: %v", c.to, s.Pos)
				}
				prev = s.Pos
			}
			if math.Abs(s.Pos-c.to) > 1e-3*math.Abs(c.to-c.from) {
				t.Errorf("after 30 frames at %v, want %v", s.Pos, c.to)
			}
		})
	}
}

func TestSpringStepReturnsACopy(t *testing.T) {
	t.Parallel()
	s := viz.NewSpring(0, 1)
	next := s.Step()
	if s.Pos != 0 || s.Vel != 0 || s.Target != 1 {
		t.Errorf("Step changed its receiver: %+v", s)
	}
	if next.Pos <= 0 || next.Vel <= 0 || next.Target != 1 {
		t.Errorf("Step did not advance: %+v", next)
	}
}

func TestSpringAtRestStaysPut(t *testing.T) {
	t.Parallel()
	s := viz.NewSpring(0.5, 0.5)
	if !s.Settled(12) {
		t.Fatal("a spring at its target should be settled")
	}
	if got := s.Step(); got.Pos != 0.5 || got.Vel != 0 {
		t.Errorf("Step at rest = %+v", got)
	}
}

// Retargeting mid-flight keeps the velocity, so a new event joins the
// running motion instead of restarting it.
func TestSpringRetargetKeepsVelocity(t *testing.T) {
	t.Parallel()
	s := viz.NewSpring(0, 1).Step().Step()
	moved := s
	moved.Target = 2
	if moved.Vel != s.Vel || moved.Pos != s.Pos {
		t.Fatal("retarget must not reset position or velocity")
	}
	if n := framesToSettle(moved, 12, 100); n > 2*viz.MaxFrames {
		t.Errorf("retargeted spring took %d frames", n)
	}
}

func TestSpringStopRuleScalesWithWidth(t *testing.T) {
	t.Parallel()
	s := viz.Spring{Pos: 0.99, Target: 1}
	cases := []struct {
		width int
		want  bool
	}{
		{width: 1, want: true},
		{width: 12, want: true},
		{width: 13, want: false},
		{width: 0, want: true},
		{width: -4, want: true},
	}
	for _, c := range cases {
		if got := s.Settled(c.width); got != c.want {
			t.Errorf("Settled(%d) at 0.99 = %v, want %v", c.width, got, c.want)
		}
	}
}

func TestFrameIntervalIsTwentyFPS(t *testing.T) {
	t.Parallel()
	if viz.FrameInterval != 50*time.Millisecond {
		t.Errorf("FrameInterval = %v, want 50ms", viz.FrameInterval)
	}
}

// The 8-frame guarantee holds at width 12. A wider bar is settled on
// min(width, SettleWidth), or a 120-cell bar would chase 1/960 for ages.
func TestSpringWidePhaseBarSettlesWhenCappedAtTwelve(t *testing.T) {
	t.Parallel()
	const width = 120
	if viz.SettleWidth != 12 {
		t.Fatalf("SettleWidth = %d, want 12", viz.SettleWidth)
	}
	if n := framesToSettle(viz.NewSpring(0, 1), min(width, viz.SettleWidth), 100); n > viz.MaxFrames {
		t.Errorf("capped 120-cell bar settled in %d frames, want <= %d", n, viz.MaxFrames)
	}
}
