package dashboard

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// chartIndent is the space before the chart: the cursor column and two.
const chartIndent = "  "

// chartLines draws the cluster CPU and memory history under the node table
// in size lines: a blank, the label line, the braille rows and the
// footnote, losing the footnote and then a braille row as size shrinks.
// Without metrics it is a blank and the reason.
func chartLines(f frame, w, size int) []string {
	if size <= 0 {
		return nil
	}
	if !hasMetrics(f.snap) {
		return []string{"", " " + chartIndent + metricsNote(f)}[:min(size, 2)]
	}
	usable := w - 1 - len(chartIndent) - 1
	half := (usable - 3) / 2
	// Both charts span the same time, so they share a width.
	half2 := half
	rows := 1
	if size >= 3+chartRows {
		rows = chartRows
	}
	n := f.snap.Nodes
	sel := f.focus.panel == dash.PanelNodes && f.focus.row == chartKey
	label := chartIndent + chartHead(f, "cluster cpu", "cpu", n.CPU, cpuText(n.CPU), half) + "   " +
		chartHead(f, "cluster memory", "mem", n.Memory, memText(n.Memory), half2)
	out := []string{"", f.listLine(label, sel)}
	cpu := strings.Split(viz.Braille(f.th, half, rows, f.hist.cpu.values(), 100), "\n")
	mem := strings.Split(viz.Braille(f.th, half2, rows, f.hist.mem.values(), 100), "\n")
	for i := range rows {
		out = append(out, " "+chartIndent+cpu[i]+"   "+mem[i])
	}
	if size >= 4 {
		out = append(out, " "+chartIndent+chartFoot(f, usable))
	}
	return out
}

// chartHead is one half's label: the measure and the span it shows on the
// left, the cluster total and percent on the right. Where that does not
// fit, the short name and the percent alone.
func chartHead(f frame, name, short string, m dash.Meter, value string, w int) string {
	span := f.ds.Approx.Render(styles.GlyphApprox + spanLabel(w))
	left := name + "  " + span
	if ansi.StringWidth(left)+2+ansi.StringWidth(value) > w {
		left, value = short+" "+span, markGlyphs[markUnknown]
		if m.Known {
			value = fmt.Sprintf("%d%%", m.Percent)
		}
	}
	return spread(left, value, w)
}

// spanLabel is the time a braille chart w cells wide covers, two metrics
// samples a cell, in whole minutes, rounded to five past ten: "30m".
func spanLabel(w int) string {
	samples := min(2*w, historyLimit)
	m := float64(samples) * metricsStep.Seconds() / 60
	if m >= 10 {
		m = math.Round(m/5) * 5
	}
	return fmt.Sprintf("%dm", int(math.Round(m)))
}

func chartFoot(f frame, w int) string {
	since := ""
	if !f.hist.firstAt.IsZero() {
		since = " since " + f.hist.firstAt.Format("15:04")
	}
	text := " history k4s builds from kubectl top nodes, one point per metrics sample (15s)" + since +
		"  ·  " + viz.HotGlyph + " 85% or more"
	return cutText(f.ds.Approx.Render(styles.GlyphApprox)+f.muted(text), w)
}

func cpuText(m dash.Meter) string {
	if !m.Known {
		return markGlyphs[markUnknown]
	}
	return fmt.Sprintf("%.1f / %s cores  %d%%", float64(m.Used)/1000, trimCores(m.Capacity), m.Percent)
}

func trimCores(milli int64) string {
	if milli%1000 == 0 {
		return fmt.Sprintf("%d", milli/1000)
	}
	return fmt.Sprintf("%.1f", float64(milli)/1000)
}

func memText(m dash.Meter) string {
	if !m.Known {
		return markGlyphs[markUnknown]
	}
	return fmt.Sprintf("%s / %s GiB  %d%%", gib(m.Used), gib(m.Capacity), m.Percent)
}

func gib(b int64) string {
	g := float64(b) / (1 << 30)
	if g >= 10 {
		return fmt.Sprintf("%.0f", g)
	}
	return fmt.Sprintf("%.1f", g)
}

// metricsNote says why there are no meters or charts.
func metricsNote(f frame) string {
	m := f.snap.Nodes.Metrics
	switch m.State {
	case dash.StateAbsent:
		hint := "install kubernetes-sigs/metrics-server"
		if f.snap.Cluster.Version.Distro == "k3s" {
			hint = "bundled with k3s; started with --disable=metrics-server?"
		}
		return f.paint(markOff) + " no metrics-server: " + hint
	case dash.StateNotReady:
		return f.paint(markOff) + " metrics not ready"
	case dash.StateForbidden:
		return f.paint(markUnknown) + " metrics forbidden · " + toKubectl(m.CanI).String()
	case dash.StateUnknown:
		return f.paint(markFact) + " waiting for metrics"
	}
	return f.paint(markOff) + " metrics " + m.State.String()
}
