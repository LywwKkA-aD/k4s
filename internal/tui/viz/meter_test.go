package viz_test

import (
	"math"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

func TestMeterShrinksFromTwelveCellsToSixToTheValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		width   int
		percent float64
		want    string
	}{
		{name: "12 cells", width: 18, percent: 38, want: "████▌·······  38% "},
		{name: "12 cells padded", width: 20, percent: 38, want: "████▌·······  38%   "},
		{name: "6 cells below 18", width: 17, percent: 38, want: "██▎···  38%      "},
		{name: "6 cells", width: 12, percent: 38, want: "██▎···  38% "},
		{name: "value below 12", width: 11, percent: 38, want: " 38%       "},
		{name: "value", width: 5, percent: 38, want: " 38% "},
		{name: "kubectl rounding 52", width: 18, percent: 52, want: "██████▎·····  52% "},
		{name: "kubectl rounding 14", width: 18, percent: 14, want: "█▋··········  14% "},
		{name: "full", width: 18, percent: 100, want: "████████████ 100%▲"},
		{name: "empty", width: 18, percent: 0, want: "············   0% "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(viz.Meter(viz.Mono(), c.width, c.percent, 1)); got != c.want {
				t.Errorf("Meter(%d, %v) = %q, want %q", c.width, c.percent, got, c.want)
			}
		})
	}
}

func TestMeterHotMarkFromEightyFive(t *testing.T) {
	t.Parallel()
	cases := []struct {
		percent float64
		hot     bool
	}{
		{percent: 84, hot: false},
		{percent: 84.4, hot: false},
		{percent: 84.5, hot: true},
		{percent: 85, hot: true},
		{percent: 87, hot: true},
	}
	for _, c := range cases {
		got := plain(viz.Meter(viz.Mono(), 18, c.percent, 1))
		if strings.HasSuffix(got, viz.HotGlyph) != c.hot {
			t.Errorf("Meter(%v) = %q, hot mark want %v", c.percent, got, c.hot)
		}
	}
}

func TestMeterLevelThresholds(t *testing.T) {
	t.Parallel()
	cases := []struct {
		percent float64
		want    viz.Level
	}{
		{percent: 0, want: viz.LevelOK},
		{percent: 69.4, want: viz.LevelOK},
		{percent: 69.5, want: viz.LevelWarn},
		{percent: 70, want: viz.LevelWarn},
		{percent: 84.4, want: viz.LevelWarn},
		{percent: 84.5, want: viz.LevelErr},
		{percent: 85, want: viz.LevelErr},
		{percent: 250, want: viz.LevelErr},
		{percent: -5, want: viz.LevelOK},
		{percent: math.NaN(), want: viz.LevelOK},
	}
	for _, c := range cases {
		if got := viz.MeterLevel(c.percent); got != c.want {
			t.Errorf("MeterLevel(%v) = %v, want %v", c.percent, got, c.want)
		}
	}
}

func TestMeterProgressScalesTheFillNotTheValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		progress float64
		want     string
	}{
		{progress: 0, want: "············  87%▲"},
		{progress: 0.5, want: "█████▎······  87%▲"},
		{progress: 1, want: "██████████▌·  87%▲"},
		{progress: 1.3, want: "██████████▌·  87%▲"},
		{progress: -1, want: "············  87%▲"},
	}
	for _, c := range cases {
		if got := plain(viz.Meter(viz.Mono(), 18, 87, c.progress)); got != c.want {
			t.Errorf("progress %v: %q, want %q", c.progress, got, c.want)
		}
	}
}

func TestMeterClampsOutOfRangeValues(t *testing.T) {
	t.Parallel()
	cases := []struct {
		percent float64
		want    string
	}{
		{percent: 150, want: "████████████ 100%▲"},
		{percent: -20, want: "············   0% "},
		{percent: math.NaN(), want: "············   0% "},
	}
	for _, c := range cases {
		if got := plain(viz.Meter(viz.Mono(), 18, c.percent, 1)); got != c.want {
			t.Errorf("Meter(%v) = %q, want %q", c.percent, got, c.want)
		}
	}
}

func TestMeterValueAtTinyWidths(t *testing.T) {
	t.Parallel()
	cases := []struct {
		width   int
		percent float64
		want    string
	}{
		{width: 4, percent: 87, want: "87%▲"},
		{width: 3, percent: 87, want: "87▲"},
		{width: 2, percent: 87, want: "87"},
		{width: 1, percent: 87, want: "…"},
		{width: 4, percent: 38, want: "38% "},
		{width: 3, percent: 38, want: "38%"},
		{width: 4, percent: 100, want: "100▲"},
		{width: 3, percent: 100, want: "100"},
		{width: 1, percent: 5, want: "5"},
	}
	for _, c := range cases {
		if got := plain(viz.Meter(viz.Mono(), c.width, c.percent, 1)); got != c.want {
			t.Errorf("Meter(%d, %v) = %q, want %q", c.width, c.percent, got, c.want)
		}
	}
}

// Mono carries the hot level through bold and underline, as the styles
// package's Err style does.
func TestMeterMonoMarksHotValueWithAttributes(t *testing.T) {
	t.Parallel()
	hot := viz.Meter(viz.Mono(), 18, 90, 1)
	if !strings.Contains(hot, "\x1b[") {
		t.Errorf("hot meter in mono has no attributes: %q", hot)
	}
	cool := viz.Meter(viz.Mono(), 18, 10, 1)
	if cool != plain(cool) {
		t.Errorf("ok meter in mono should be plain text: %q", cool)
	}
}
