package viz_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// cellsOf counts the cells of each phase glyph in a stripped bar.
func cellsOf(bar string) map[viz.Phase]int {
	out := map[viz.Phase]int{}
	for _, r := range bar {
		for p := viz.PhaseRunning; p <= viz.PhaseOther; p++ {
			if string(r) == p.Glyph() {
				out[p]++
			}
		}
	}
	return out
}

func TestPhaseBarSharesCellsByLargestRemainder(t *testing.T) {
	t.Parallel()
	got := cellsOf(plain(viz.PhaseBar(viz.Mono(), 120, busyPhases(), 1)))
	want := map[viz.Phase]int{
		viz.PhaseRunning: 111, viz.PhaseCompleted: 4, viz.PhasePending: 1,
		viz.PhaseFailing: 3, viz.PhaseOther: 1,
	}
	for p, n := range want {
		if got[p] != n {
			t.Errorf("%s: %d cells, want %d (all: %v)", p, got[p], n, got)
		}
	}
}

func TestPhaseBar(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		width    int
		counts   []viz.PhaseCount
		progress float64
		want     string
	}{
		{
			name:  "phases in fixed order",
			width: 5,
			counts: []viz.PhaseCount{
				{Phase: viz.PhaseOther, Count: 1}, {Phase: viz.PhaseFailing, Count: 1},
				{Phase: viz.PhasePending, Count: 1}, {Phase: viz.PhaseCompleted, Count: 1},
				{Phase: viz.PhaseRunning, Count: 1},
			},
			progress: 1, want: "█▓░▚·",
		},
		{
			name:     "a lone failing pod keeps a cell",
			width:    10,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseRunning, Count: 1000}, {Phase: viz.PhaseFailing, Count: 1}},
			progress: 1, want: "█████████▚",
		},
		{
			name:     "narrower than the phases keeps the most severe",
			width:    2,
			counts:   busyPhases(),
			progress: 1, want: "░▚",
		},
		{
			name:     "entries of one phase add up",
			width:    4,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseFailing, Count: 1, Label: "A"}, {Phase: viz.PhaseFailing, Count: 1, Label: "B"}, {Phase: viz.PhaseRunning, Count: 2}},
			progress: 1, want: "██▚▚",
		},
		{
			name:     "progress reveals from the left",
			width:    10,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseRunning, Count: 10}},
			progress: 0.5, want: "█████     ",
		},
		{
			name:     "progress zero is blank",
			width:    4,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseRunning, Count: 10}},
			progress: 0, want: "    ",
		},
		{
			name:     "negative counts are ignored",
			width:    4,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseRunning, Count: 2}, {Phase: viz.PhaseFailing, Count: -3}},
			progress: 1, want: "████",
		},
		{
			name:     "unknown phase counts as other",
			width:    2,
			counts:   []viz.PhaseCount{{Phase: viz.PhaseRunning, Count: 1}, {Phase: viz.Phase(42), Count: 1}},
			progress: 1, want: "█·",
		},
		{name: "no pods", width: 3, counts: nil, progress: 1, want: "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(viz.PhaseBar(viz.Mono(), c.width, c.counts, c.progress)); got != c.want {
				t.Errorf("PhaseBar = %q, want %q", got, c.want)
			}
		})
	}
}

func TestPhaseLegend(t *testing.T) {
	t.Parallel()
	words := "█ Running 171   ▓ Completed 7   ░ Pending/NotReady 2   ▚ CrashLoopBackOff 3 · ImagePullBackOff 1   · Evicted 1"
	tight := strings.ReplaceAll(words, "   ", "  ")
	totals := "█171 ▓7 ░2 ▚4 ·1"
	cases := []struct {
		name  string
		width int
		want  string
	}{
		{name: "words", width: len([]rune(words)) + 2, want: words + "  "},
		{name: "tighter gaps", width: len([]rune(tight)), want: tight},
		{name: "totals", width: len([]rune(tight)) - 1, want: totals + strings.Repeat(" ", len([]rune(tight))-1-len([]rune(totals)))},
		{name: "totals exact", width: len([]rune(totals)), want: totals},
		{name: "last entry dropped", width: 15, want: "█171 ▓7 ░2 ▚4 …"},
		{name: "whole entries dropped", width: 14, want: "█171 ▓7 ░2 …  "},
		{name: "never cuts a count", width: 8, want: "█171 …  "},
		{name: "ellipsis only", width: 5, want: "…    "},
		{name: "one column", width: 1, want: "…"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(viz.PhaseLegend(viz.Mono(), c.width, busyPhases())); got != c.want {
				t.Errorf("PhaseLegend(%d) = %q, want %q", c.width, got, c.want)
			}
		})
	}
}

func TestPhaseLegendNamesPhasesWithoutLabel(t *testing.T) {
	t.Parallel()
	counts := []viz.PhaseCount{
		{Phase: viz.PhaseFailing, Count: 2},
		{Phase: viz.PhaseRunning, Count: 0},
		{Phase: viz.PhaseOther, Count: 1},
	}
	want := "▚ Failing 2   · Other 1"
	if got := plain(viz.PhaseLegend(viz.Mono(), len([]rune(want)), counts)); got != want {
		t.Errorf("legend = %q, want %q", got, want)
	}
	if got := plain(viz.PhaseLegend(viz.Mono(), 3, nil)); got != "   " {
		t.Errorf("empty legend = %q, want blanks", got)
	}
}

func TestPhaseGlyphsAndNamesAreDistinct(t *testing.T) {
	t.Parallel()
	glyphs, names := map[string]bool{}, map[string]bool{}
	for p := viz.PhaseRunning; p <= viz.PhaseOther; p++ {
		glyphs[p.Glyph()] = true
		names[p.String()] = true
	}
	if len(glyphs) != 5 || len(names) != 5 {
		t.Errorf("glyphs %v, names %v: want 5 distinct of each", glyphs, names)
	}
	if viz.Phase(-1).Glyph() != viz.PhaseOther.Glyph() || viz.Phase(9).String() != viz.PhaseOther.String() {
		t.Error("an unknown phase should read as other")
	}
}
