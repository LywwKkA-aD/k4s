package viz

import (
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Phase is a group of pods on the phase bar.
type Phase int

// Phases in bar order. Each has its own glyph, so the bar reads the same
// without colour.
const (
	// PhaseRunning is drawn █ in OK.
	PhaseRunning Phase = iota
	// PhaseCompleted is drawn ▓ in Muted.
	PhaseCompleted
	// PhasePending covers Pending and not ready, drawn ░ in Warn.
	PhasePending
	// PhaseFailing covers CrashLoopBackOff, ImagePullBackOff and the like,
	// drawn ▚ in Err.
	PhaseFailing
	// PhaseOther covers Evicted and anything unclassified, drawn · in Muted.
	PhaseOther
	phaseCount
)

var (
	phaseGlyphs = [phaseCount]string{"█", "▓", "░", "▚", "·"}
	phaseNames  = [phaseCount]string{"Running", "Completed", "Pending/NotReady", "Failing", "Other"}
	// bySeverity decides which phases keep a cell on a bar narrower than
	// the number of phases with pods.
	bySeverity = [phaseCount]Phase{PhaseFailing, PhasePending, PhaseOther, PhaseCompleted, PhaseRunning}
)

// known maps an out-of-range phase to PhaseOther.
func (p Phase) known() Phase {
	if p < 0 || p >= phaseCount {
		return PhaseOther
	}
	return p
}

// Glyph returns the bar and legend glyph of p.
func (p Phase) Glyph() string { return phaseGlyphs[p.known()] }

// String returns the legend name of p.
func (p Phase) String() string { return phaseNames[p.known()] }

func (p Phase) style(th Theme) lipgloss.Style {
	switch p.known() {
	case PhaseRunning:
		return th.OK
	case PhasePending:
		return th.Warn
	case PhaseFailing:
		return th.Err
	}
	return th.Muted
}

// PhaseCount is one entry of the phase bar and its legend. Entries of one
// phase add up on the bar and share a glyph in the legend.
type PhaseCount struct {
	Phase Phase
	Count int
	// Label names the entry in the legend, such as a reason
	// ("CrashLoopBackOff"); empty means the phase name.
	Label string
}

// PhaseBar draws counts as one bar exactly width cells wide, phases in bar
// order. Every phase with pods keeps at least one cell while width allows,
// so one failing pod among hundreds still shows; the rest is shared by
// largest remainder. progress (0 to 1) reveals the bar from the left, for
// the spring. The unrevealed part is blank, and so is a bar with no pods.
func PhaseBar(th Theme, width int, counts []PhaseCount, progress float64) string {
	if width <= 0 {
		return ""
	}
	cells := apportion(phaseTotals(counts), width)
	left := int(math.Round(within(progress, 1) * float64(width)))
	var b strings.Builder
	for p, n := range cells {
		n = min(n, left)
		if n <= 0 {
			continue
		}
		b.WriteString(paint(Phase(p).style(th), strings.Repeat(phaseGlyphs[p], n)))
		left -= n
	}
	return pad(b.String(), width)
}

// PhaseLegend lists the phases with pods in bar order, exactly width
// columns: "█ Running 171   ▚ CrashLoopBackOff 3 · ImagePullBackOff 1".
// When that does not fit it tightens the gaps, then falls back to glyphs
// and totals ("█171 ▚4"), then drops whole entries from the end behind an
// ellipsis. A count is never cut, so no number reads smaller than it is.
func PhaseLegend(th Theme, width int, counts []PhaseCount) string {
	if width <= 0 {
		return ""
	}
	groups := legendGroups(counts)
	for _, gap := range []string{"   ", "  "} {
		if s := legendWords(th, groups, gap); ansi.StringWidth(s) <= width {
			return pad(s, width)
		}
	}
	return pad(fitEntries(legendTotals(th, groups), width), width)
}

func phaseTotals(counts []PhaseCount) []int {
	totals := make([]int, phaseCount)
	for _, c := range counts {
		if c.Count > 0 {
			totals[c.Phase.known()] += c.Count
		}
	}
	return totals
}

// legendGroups collects the entries with pods by phase, in input order
// within a phase.
func legendGroups(counts []PhaseCount) [phaseCount][]PhaseCount {
	var groups [phaseCount][]PhaseCount
	for _, c := range counts {
		if c.Count > 0 {
			p := c.Phase.known()
			groups[p] = append(groups[p], c)
		}
	}
	return groups
}

func legendWords(th Theme, groups [phaseCount][]PhaseCount, gap string) string {
	parts := make([]string, 0, phaseCount)
	for p, entries := range groups {
		if len(entries) == 0 {
			continue
		}
		words := make([]string, len(entries))
		for i, e := range entries {
			label := e.Label
			if label == "" {
				label = Phase(p).String()
			}
			words[i] = label + " " + strconv.Itoa(e.Count)
		}
		parts = append(parts, paint(Phase(p).style(th), phaseGlyphs[p])+" "+strings.Join(words, " · "))
	}
	return strings.Join(parts, gap)
}

func legendTotals(th Theme, groups [phaseCount][]PhaseCount) []string {
	parts := make([]string, 0, phaseCount)
	for p, entries := range groups {
		total := 0
		for _, e := range entries {
			total += e.Count
		}
		if total > 0 {
			parts = append(parts, paint(Phase(p).style(th), phaseGlyphs[p])+strconv.Itoa(total))
		}
	}
	return parts
}

// fitEntries joins parts with spaces, keeping as many whole parts from the
// front as fit in width followed by an ellipsis.
func fitEntries(parts []string, width int) string {
	for n := len(parts); n > 0; n-- {
		s := strings.Join(parts[:n], " ")
		if n < len(parts) {
			s += " " + ellipsis
		}
		if ansi.StringWidth(s) <= width {
			return s
		}
	}
	return ellipsis
}

// apportion splits width cells over totals in proportion. Every non-zero
// total gets at least one cell, then cells move by largest remainder: a
// cell goes to the total furthest below its exact share, and comes off the
// one furthest above it. When width is smaller than the number of non-zero
// totals, the most severe phases keep one cell each.
func apportion(totals []int, width int) []int {
	out := make([]int, len(totals))
	sum, nonzero := 0, 0
	for _, t := range totals {
		sum += t
		if t > 0 {
			nonzero++
		}
	}
	if sum == 0 {
		return out
	}
	if width < nonzero {
		kept := 0
		for _, p := range bySeverity {
			if kept < width && totals[p] > 0 {
				out[p] = 1
				kept++
			}
		}
		return out
	}
	given := 0
	for i, t := range totals {
		if t > 0 {
			out[i] = max(1, t*width/sum)
			given += out[i]
		}
	}
	for ; given < width; given++ {
		out[furthest(totals, out, width, sum, true)]++
	}
	for ; given > width; given-- {
		out[furthest(totals, out, width, sum, false)]--
	}
	return out
}

// furthest returns the non-zero total whose cells sit furthest below its
// exact share (below) or above it, the first one on a tie. Going above, a
// total down to its last cell is skipped, so no phase with pods vanishes.
func furthest(totals, out []int, width, sum int, below bool) int {
	best, bestScore := -1, 0
	for i, t := range totals {
		if t == 0 || (!below && out[i] <= 1) {
			continue
		}
		score := t*width - out[i]*sum
		if !below {
			score = -score
		}
		if best < 0 || score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}
