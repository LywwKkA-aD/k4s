package viz

import (
	"math"
	"time"
)

// FrameInterval is the fixed step of the animation clock: one Step per
// frame, 20 frames a second.
const FrameInterval = 50 * time.Millisecond

// MaxFrames is the frame budget of one animation event. A spring still
// short of its stop rule after that many frames snaps to its target.
const MaxFrames = 8

// SpringOmega is the angular frequency of the spring, in radians per
// second. The exact critically damped curve needs 10 frames from 0 to 1 at
// the ω≈14 first sketched; 17 meets the 8-frame budget at width 12 with
// room to spare, and still eases rather than jumps.
const SpringOmega = 17.0

// SettleWidth is the widest width the 8-frame budget holds for: the stop
// rule gets stricter as 1/(8·width), so a 120-cell phase bar chasing 1/960
// would run well past MaxFrames. Callers settle on
// Settled(min(width, SettleWidth)).
const SettleWidth = 12

// Spring is a critically damped spring (ζ = 1): it eases from Pos to
// Target without overshooting when it starts at rest. Retarget it by
// setting Target; the velocity carries over, so a new event joins a
// running motion instead of restarting it.
type Spring struct {
	Pos, Vel, Target float64
}

// NewSpring returns a spring at rest at from, heading for to.
func NewSpring(from, to float64) Spring {
	return Spring{Pos: from, Target: to}
}

// Step returns the spring one FrameInterval later. It evaluates the exact
// solution x(t) = (x0 + (v0 + ω·x0)·t)·e^(−ωt) rather than integrating, so
// the fixed 50 ms step stays stable and every run draws the same frames.
func (s Spring) Step() Spring {
	dt := FrameInterval.Seconds()
	x0 := s.Pos - s.Target
	c := s.Vel + SpringOmega*x0
	decay := math.Exp(-SpringOmega * dt)
	return Spring{
		Pos:    s.Target + (x0+c*dt)*decay,
		Vel:    (s.Vel - SpringOmega*c*dt) * decay,
		Target: s.Target,
	}
}

// Settled reports whether the spring is close enough to Target that a
// widget width cells wide, drawn in eighth blocks, no longer changes:
// |Pos − Target| < 1/(8·width). Positions are fractions of the full bar.
// The MaxFrames guarantee is for width 12: pass min(width, SettleWidth).
func (s Spring) Settled(width int) bool {
	return math.Abs(s.Pos-s.Target) < 1/(8*float64(max(width, 1)))
}
