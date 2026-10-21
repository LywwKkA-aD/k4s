package dashboard

import (
	"fmt"
	"math"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const (
	// sparkBelow is the braille half-width under which the cluster history
	// becomes two spark rows, one cell a minute (the 120x40 mockup):
	// braille that narrow would span only a few minutes.
	sparkBelow = 40
	// sparkLeadW is "cpu ≈25m ", the name and span before a spark row.
	sparkLeadW = 9
)

// sparkLines is the cluster history as two rows, CPU over memory, each a
// minute a cell with the cluster total on the right, then the footnote
// where size allows. w is the room after the indent.
func sparkLines(f frame, w, size int) []string {
	n := f.snap.Nodes
	cpu, mem := sparkTotal(n.CPU, "cores", trimCores), sparkTotal(n.Memory, "GiB", gib)
	right := max(ansi.StringWidth(cpu), ansi.StringWidth(mem))
	cells := max(w-sparkLeadW-2-right, 1)
	span := f.ds.Approx.Render(padRight(styles.GlyphApprox+minutesLabel(cells), sparkLeadW-len("cpu  ")))
	row := func(name string, m clockMinutes, total string) string {
		return name + " " + span + " " + viz.Spark(f.th, cells, m.values(), 100) + "  " + padRight(total, right)
	}
	sel := f.focus.panel == dash.PanelNodes && f.focus.row == chartKey
	out := []string{
		"",
		f.listLine(chartIndent+row("cpu", f.hist.cpuMin, cpu), sel),
		" " + chartIndent + row("mem", f.hist.memMin, mem),
	}
	if size >= 4 {
		foot := f.ds.Approx.Render(styles.GlyphApprox) + f.muted(" history from kubectl top nodes · "+viz.HotGlyph+" ≥ 85%")
		out = append(out, " "+chartIndent+cutText(foot, w))
	}
	return out
}

// sparkTotal is "36% of 36 cores", "68% of 192 GiB".
func sparkTotal(m dash.Meter, unit string, amount func(int64) string) string {
	if !m.Known {
		return markGlyphs[markUnknown]
	}
	return fmt.Sprintf("%d%% of %s %s", m.Percent, amount(m.Capacity), unit)
}

// minutesLabel is a strip of one-minute cells, in whole minutes, rounded
// to five past ten: "25m".
func minutesLabel(cells int) string {
	m := float64(min(cells, historyLimit/int(time.Minute/metricsStep)))
	if m >= 10 {
		m = math.Round(m/5) * 5
	}
	return fmt.Sprintf("%dm", int(m))
}
