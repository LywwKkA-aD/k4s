package viz_test

import (
	"math"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

var gap = math.NaN()

func TestSparkScalesSamplesToBlocks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		width  int
		series []float64
		maxV   float64
		want   string
	}{
		{name: "zero to max", width: 3, series: []float64{0, 50, 100}, maxV: 100, want: "▁▅█"},
		{name: "every level", width: 8, series: []float64{0, 1, 2, 3, 4, 5, 6, 7}, maxV: 7, want: "▁▂▃▄▅▆▇█"},
		{name: "gap is blank", width: 3, series: []float64{1, gap, 1}, maxV: 0, want: "█ █"},
		{name: "short history padded left", width: 5, series: []float64{0, 100}, maxV: 100, want: "   ▁█"},
		{name: "newest samples kept", width: 2, series: []float64{0, 50, 100}, maxV: 100, want: "▅█"},
		{name: "auto max", width: 3, series: []float64{1, 2, 4}, maxV: 0, want: "▃▅█"},
		{name: "all zero", width: 2, series: []float64{0, 0}, maxV: 0, want: "▁▁"},
		{name: "above max clamps", width: 2, series: []float64{-3, 300}, maxV: 100, want: "▁█"},
		{name: "infinite sample ignored by auto max", width: 2, series: []float64{2, math.Inf(1)}, maxV: 0, want: "██"},
		{name: "no history", width: 3, series: nil, maxV: 100, want: "   "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(viz.Spark(viz.Mono(), c.width, c.series, c.maxV)); got != c.want {
				t.Errorf("Spark = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBrailleCellsHoldTwoSamplesOfFourDots(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		width  int
		height int
		series []float64
		want   string
	}{
		{name: "zero is a baseline dot", width: 1, height: 1, series: []float64{0, 0}, want: "⣀"},
		{name: "left low right full", width: 1, height: 1, series: []float64{0, 100}, want: "⣸"},
		{name: "left full right low", width: 1, height: 1, series: []float64{100, 0}, want: "⣇"},
		{name: "half", width: 1, height: 1, series: []float64{50, 50}, want: "⣤"},
		{name: "gap pair is blank", width: 2, height: 1, series: []float64{100, 100, gap, gap}, want: "⣿ "},
		{name: "gap in one column", width: 1, height: 1, series: []float64{gap, 100}, want: "⢸"},
		{name: "short history padded left", width: 2, height: 1, series: []float64{100}, want: " ⢸"},
		{name: "two rows full", width: 1, height: 2, series: []float64{100, 100}, want: "⣿\n⣿"},
		{name: "two rows half", width: 1, height: 2, series: []float64{50, 50}, want: " \n⣿"},
		{name: "two rows three quarters", width: 1, height: 2, series: []float64{75, 25}, want: "⡄\n⣧"},
		{name: "newest samples kept", width: 1, height: 1, series: []float64{100, 100, 0, 0}, want: "⣀"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(viz.Braille(viz.Mono(), c.width, c.height, c.series, 100)); got != c.want {
				t.Errorf("Braille = %q, want %q", got, c.want)
			}
		})
	}
}

func TestBrailleAutoScale(t *testing.T) {
	t.Parallel()
	got := plain(viz.Braille(viz.Mono(), 1, 1, []float64{2, 4}, 0))
	if got != "⣼" {
		t.Errorf("auto-scaled braille = %q, want %q", got, "⣼")
	}
}

func TestSparkPaintsInAccent2(t *testing.T) {
	t.Parallel()
	th := viz.Mono()
	th.Accent2 = th.Accent2.Bold(true)
	for name, s := range map[string]string{
		"spark":   viz.Spark(th, 4, []float64{1, 2}, 0),
		"braille": viz.Braille(th, 4, 1, []float64{1, 2}, 0),
	} {
		if !strings.Contains(s, "\x1b[1m") {
			t.Errorf("%s not painted with Accent2: %q", name, s)
		}
	}
}
