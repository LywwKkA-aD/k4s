package viz_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const maxGoldenWidth = 40

// widget is one golden case: a render at any width, height lines tall.
type widget struct {
	name   string
	height int
	render func(th viz.Theme, width int) string
}

func busyPhases() []viz.PhaseCount {
	return []viz.PhaseCount{
		{Phase: viz.PhaseRunning, Count: 171},
		{Phase: viz.PhaseCompleted, Count: 7},
		{Phase: viz.PhasePending, Count: 2},
		{Phase: viz.PhaseFailing, Count: 3, Label: "CrashLoopBackOff"},
		{Phase: viz.PhaseFailing, Count: 1, Label: "ImagePullBackOff"},
		{Phase: viz.PhaseOther, Count: 1, Label: "Evicted"},
	}
}

func clearPhases() []viz.PhaseCount {
	return []viz.PhaseCount{
		{Phase: viz.PhaseRunning, Count: 178},
		{Phase: viz.PhaseCompleted, Count: 7},
	}
}

// history is a deterministic 90-sample series with a gap where the user
// was away from the dashboard.
func history() []float64 {
	out := make([]float64, 90)
	for i := range out {
		out[i] = float64((i*37)%101) * 0.9
	}
	for i := 50; i < 58; i++ {
		out[i] = math.NaN()
	}
	return out
}

func meterCase(percent, progress float64) widget {
	name := fmt.Sprintf("meter_%03.0f", percent)
	if progress < 1 {
		name += "_half"
	}
	return widget{name: name, height: 1, render: func(th viz.Theme, w int) string {
		return viz.Meter(th, w, percent, progress)
	}}
}

func widgets() []widget {
	hist := history()
	cluster := viz.Box{Title: "1 cluster", Info: "◆ k3s", Footer: "ctx demo"}
	pods := viz.Box{Title: "3 pods", Info: "185 · 33 ns", Footer: "1/9 · +2 · z zoom", Focused: true}
	clusterBody := " ● api      ready · 8/8 checks\n ◆ version  v1.36.3+k3s1 · embedded etcd"
	podsBody := "›▲ shop/payments-worker-6c9f7d8b5-xk2lp  0/1  CrashLoopBackOff\n △ ci/runner  0/1  Pending\n · growlytics/report  Evicted\n ◇ extra row past the height"
	return []widget{
		meterCase(0, 1), meterCase(38, 1), meterCase(71, 1), meterCase(87, 1), meterCase(100, 1), meterCase(87, 0.5),
		{name: "spark", height: 1, render: func(th viz.Theme, w int) string { return viz.Spark(th, w, hist, 100) }},
		{name: "spark_auto", height: 1, render: func(th viz.Theme, w int) string { return viz.Spark(th, w, hist[:20], 0) }},
		{name: "braille_1", height: 1, render: func(th viz.Theme, w int) string { return viz.Braille(th, w, 1, hist, 100) }},
		{name: "braille_2", height: 2, render: func(th viz.Theme, w int) string { return viz.Braille(th, w, 2, hist, 100) }},
		{name: "phasebar_busy", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseBar(th, w, busyPhases(), 1) }},
		{name: "phasebar_clear", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseBar(th, w, clearPhases(), 1) }},
		{name: "phasebar_half", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseBar(th, w, busyPhases(), 0.5) }},
		{name: "phasebar_empty", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseBar(th, w, nil, 1) }},
		{name: "legend_busy", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseLegend(th, w, busyPhases()) }},
		{name: "legend_clear", height: 1, render: func(th viz.Theme, w int) string { return viz.PhaseLegend(th, w, clearPhases()) }},
		{name: "box", height: 5, render: func(th viz.Theme, w int) string { return cluster.Render(th, w, 5, clusterBody) }},
		{name: "box_focused", height: 5, render: func(th viz.Theme, w int) string { return pods.Render(th, w, 5, podsBody) }},
	}
}

// TestWidgetGoldens pins every widget at widths 1 to 40 as mono shows it.
// Regenerate with
//
//	go test ./internal/tui/viz -run TestWidgetGoldens -update
func TestWidgetGoldens(t *testing.T) {
	t.Parallel()
	for _, wg := range widgets() {
		t.Run(wg.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			for w := 1; w <= maxGoldenWidth; w++ {
				for i, line := range strings.Split(plain(wg.render(viz.Mono(), w)), "\n") {
					label := fmt.Sprintf("%02d", w)
					if i > 0 {
						label = "  "
					}
					fmt.Fprintf(&b, "%s|%s|\n", label, line)
				}
			}
			golden(t, wg.name, b.String())
		})
	}
}

// TestWidgetsFillTheirBlock checks every render is exactly width columns
// by height lines, in both themes.
func TestWidgetsFillTheirBlock(t *testing.T) {
	t.Parallel()
	for _, wg := range widgets() {
		t.Run(wg.name, func(t *testing.T) {
			t.Parallel()
			for w := 1; w <= maxGoldenWidth; w++ {
				assertBlock(t, wg.render(viz.Mono(), w), w, wg.height)
				assertBlock(t, wg.render(colour(), w), w, wg.height)
			}
		})
	}
}

// TestColourStrippedEqualsMono is the colour-blind property: no state may
// live in colour alone, so colour output without its escape codes is the
// mono output byte for byte, and mono itself sets no colour.
func TestColourStrippedEqualsMono(t *testing.T) {
	t.Parallel()
	for _, wg := range widgets() {
		t.Run(wg.name, func(t *testing.T) {
			t.Parallel()
			for w := 1; w <= maxGoldenWidth; w++ {
				mono, col := wg.render(viz.Mono(), w), wg.render(colour(), w)
				if plain(col) != plain(mono) {
					t.Errorf("width %d: colour %q, mono %q", w, plain(col), plain(mono))
				}
				if codes := colourCodes(mono); len(codes) > 0 {
					t.Errorf("width %d: mono output sets colours %v", w, codes)
				}
			}
		})
	}
}

func TestColourThemeEmitsColour(t *testing.T) {
	t.Parallel()
	if codes := colourCodes(viz.Meter(colour(), 18, 87, 1)); len(codes) == 0 {
		t.Fatal("the colour theme painted nothing, so the stripped-equals-mono property proves nothing")
	}
}

func TestZeroSizeRendersNothing(t *testing.T) {
	t.Parallel()
	th := viz.Mono()
	got := map[string]string{
		"meter":     viz.Meter(th, 0, 50, 1),
		"spark":     viz.Spark(th, 0, history(), 100),
		"braille":   viz.Braille(th, 0, 2, history(), 100),
		"braille h": viz.Braille(th, 10, 0, history(), 100),
		"phase bar": viz.PhaseBar(th, -1, busyPhases(), 1),
		"legend":    viz.PhaseLegend(th, 0, busyPhases()),
		"box":       viz.Box{Title: "x"}.Render(th, 0, 3, "body"),
		"box h":     viz.Box{Title: "x"}.Render(th, 10, 0, "body"),
	}
	for name, s := range got {
		if s != "" {
			t.Errorf("%s at zero size = %q, want empty", name, s)
		}
	}
}
