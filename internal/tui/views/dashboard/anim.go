package dashboard

import (
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// Motion (§6) runs on one clock: a single animFrameMsg every
// viz.FrameInterval, armed only while something moves. Every event has a
// frame budget, and an event during motion joins the running clock
// instead of starting a second one. Values that change between polls
// snap; only the first snapshot springs up. On an idle cluster nothing
// moves, so the dashboard asks for no frame at all.

const (
	// sweepFrames is the budget of a gradient sweep across the focused
	// border or the verdict word.
	sweepFrames = 6
	// spinAfter is how long a refresh asked for with r, or the first
	// load, may take before the spinner replaces the heartbeat.
	spinAfter = 150 * time.Millisecond
)

// heartbeat steps once per snapshot that showed something new.
var heartbeat = [...]string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// animFrameMsg is one tick of the motion clock, for the arm that tok
// names: a frame from another dashboard, or one armed before a halt, is
// dropped.
type animFrameMsg struct{ tok async.Token }

// spinDueMsg says a refresh has run spinAfter: the spinner starts if it
// still runs.
type spinDueMsg struct{ tok async.Token }

// motion is what moves on the clock. It is a value: every step returns a
// new one.
type motion struct {
	mint async.Token
	// armed is the frame in flight, zero while the clock is stopped;
	// spin is the spinDueMsg the spinner waits for.
	armed, spin async.Token

	spring     viz.Spring
	springLeft int
	sweepLeft  int
	sweepPanel dash.Panel
	verdict    int // frames left of the verdict word's sweep
	wordmark   int // frames left of the wordmark's shimmer
	spinning   bool
	spinTicks  int
}

func newMotion() motion {
	mint := async.New()
	return motion{mint: mint, spin: mint.Next(), spring: viz.NewSpring(1, 1)}
}

// moving reports something that needs another frame.
func (mo motion) moving() bool {
	return mo.springLeft > 0 || mo.sweepLeft > 0 || mo.verdict > 0 || mo.wordmark > 0 || mo.spinning
}

// step is one frame later: the spring moves until a widget SettleWidth
// cells wide would draw it the same, or its budget runs out, and then
// snaps; every sweep loses a frame.
func (mo motion) step() motion {
	if mo.springLeft > 0 {
		mo.spring = mo.spring.Step()
		mo.springLeft--
		if mo.springLeft == 0 || mo.spring.Settled(viz.SettleWidth) {
			mo.spring, mo.springLeft = viz.NewSpring(mo.spring.Target, mo.spring.Target), 0
		}
	}
	mo.sweepLeft = max(mo.sweepLeft-1, 0)
	mo.verdict = max(mo.verdict-1, 0)
	mo.wordmark = max(mo.wordmark-1, 0)
	if mo.spinning {
		mo.spinTicks++
	}
	return mo
}

// halt stops everything where it would end: the spring at its target,
// no sweep, no spinner.
func (mo motion) halt() motion {
	return motion{mint: mo.mint, spin: mo.spin, spring: viz.NewSpring(mo.spring.Target, mo.spring.Target)}
}

// springUp starts the bars and counts from zero, as on a first snapshot.
func (mo motion) springUp() motion {
	mo.spring, mo.springLeft = viz.NewSpring(0, 1), viz.MaxFrames
	return mo
}

// beatGlyph is the heartbeat at step beat, or the spinner's frame while
// it runs.
func (mo motion) beatGlyph(beat int) string {
	if mo.spinning {
		frames := spinner.MiniDot.Frames
		at := time.Duration(mo.spinTicks) * viz.FrameInterval
		return frames[int(at/spinner.MiniDot.FPS)%len(frames)]
	}
	return heartbeat[beat%len(heartbeat)]
}

// still reports motion switched off: a terminal without colour (the
// ASCII or NoTTY profile), a terminal in the background, or renders that
// ran slow this session.
func (m Model) still() bool {
	return stillProfile(m.profile) || m.blurred || m.mem.slowView
}

func stillProfile(p colorprofile.Profile) bool {
	return p == colorprofile.NoTTY || p == colorprofile.ASCII
}

// glows reports whether gradients may run: mono has no colour to sweep,
// and its heavy border shows focus at once.
func (m Model) glows() bool {
	return !m.still() && !styles.Mono()
}

// animate arms the clock when something moves and no frame is in flight.
func (m Model) animate() (Model, tea.Cmd) {
	if !m.mo.moving() || m.mo.armed != (async.Token{}) {
		return m, nil
	}
	m.mo.mint = m.mo.mint.Next()
	m.mo.armed = m.mo.mint
	tok := m.mo.armed
	return m, m.frames.Tick(viz.FrameInterval, func(time.Time) tea.Msg { return animFrameMsg{tok: tok} })
}

func (m Model) onFrame(msg animFrameMsg) (views.View, tea.Cmd) {
	if !m.mo.armed.Accept(msg.tok) {
		return m, nil
	}
	m.mo.armed = async.Token{}
	if m.perf.slow() {
		m.mem.slowView = true
	}
	if m.still() {
		m.mo = m.mo.halt()
		return m, nil
	}
	m.mo = m.mo.step()
	return m.animate()
}

// spinLater arms the check that starts the spinner if the refresh just
// started is still running after spinAfter.
func (m Model) spinLater() (Model, tea.Cmd) {
	if m.still() {
		return m, nil
	}
	m.mo.mint = m.mo.mint.Next()
	m.mo.spin = m.mo.mint
	return m, m.spinCmd()
}

func (m Model) spinCmd() tea.Cmd {
	tok := m.mo.spin
	return m.frames.Tick(spinAfter, func(time.Time) tea.Msg { return spinDueMsg{tok: tok} })
}

func (m Model) onSpinDue(msg spinDueMsg) (views.View, tea.Cmd) {
	if !m.mo.spin.Accept(msg.tok) || m.still() || (!m.forced && m.mem.loaded) {
		return m, nil
	}
	m.mo.spinning, m.mo.spinTicks = true, 0
	return m.animate()
}

// sweepFocus starts the gradient sweep along the border of panel p, newly
// focused.
func (m Model) sweepFocus(p dash.Panel) (Model, tea.Cmd) {
	if !m.glows() || p == dash.PanelOverview {
		return m, nil
	}
	if pl, ok := m.lay.placeOf(p); !ok || pl.line {
		return m, nil
	}
	m.mo.sweepLeft, m.mo.sweepPanel = sweepFrames, p
	return m.animate()
}

// moved is what a new snapshot set in motion: the spring on the first,
// with the wordmark's shimmer in the wide layout; a sweep across the
// verdict word when it flips between attention and all clear; a heartbeat
// step when anything changed. A refresh that answered ends the spinner.
func (m Model) moved(first bool, prev, next dash.Snapshot) (Model, tea.Cmd) {
	m.mo.spinning = false
	if first || prev.Hash != next.Hash {
		m.mem.beat++
	}
	switch {
	case m.still():
	case first:
		m.mo = m.mo.springUp()
		if m.glows() && m.lay.kind == kindWide {
			m.mo.wordmark = viz.MaxFrames
		}
	case prev.Verdict.AllClear != next.Verdict.AllClear && m.glows():
		m.mo.verdict = sweepFrames
	}
	return m.animate()
}
