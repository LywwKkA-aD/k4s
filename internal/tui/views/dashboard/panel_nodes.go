package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const (
	// chartKey names the history chart's row in panel 2.
	chartKey = "chart"
	// meterNarrow is the 6-cell meter form (see viz.Meter).
	meterNarrow = 12
)

// nodeRows are panel 2's rows: one per node, then the history chart when
// there are metrics to chart.
func nodeRows(s dash.Snapshot) []row {
	out := make([]row, 0, len(s.Nodes.Rows)+1)
	for _, n := range s.Nodes.Rows {
		out = append(out, row{key: n.Key.String(), cmd: n.Cmd, target: n.Key, placed: true})
	}
	if hasMetrics(s) {
		out = append(out, row{key: chartKey, cmd: s.Nodes.TopCmd})
	}
	return out
}

// hasMetrics reports node metrics to draw: the API answers and there are
// nodes.
func hasMetrics(s dash.Snapshot) bool {
	return readable(s.Nodes.Metrics) && readable(s.Nodes.Avail) && len(s.Nodes.Rows) > 0
}

// meterCell is a node meter, or "—" when the node has no metrics. The
// meter is drawn at the data layer's whole percent, so its △ appears
// exactly when the node counts as degraded.
func meterCell(f frame, m dash.Meter, w int) string {
	if !m.Known {
		return padRight(f.paint(markUnknown), w)
	}
	return viz.Meter(f.th, w, float64(m.Percent), 1)
}

// nodesSummary is panel 2 on one line: the Ready nodes, the cluster meters
// and how many nodes are degraded.
func nodesSummary(f frame) string {
	n := f.snap.Nodes
	switch {
	case !readable(n.Avail):
		return f.unreadable("nodes", n.Avail)
	case n.Total == 0:
		return f.paint(markOff) + " no nodes registered"
	}
	worst, waiting := dash.SevOK, 0
	for _, r := range n.Rows {
		worst = max(worst, r.Sev)
		if r.Sev == dash.SevWaiting {
			waiting++
		}
	}
	parts := []string{f.paint(markOf(worst)) + fmt.Sprintf(" %d/%d Ready", n.Ready, n.Total)}
	if n.CPU.Known {
		parts = append(parts, "cpu "+strings.TrimRight(meterCell(f, n.CPU, meterNarrow), " "))
	}
	if n.Memory.Known {
		parts = append(parts, "mem "+strings.TrimRight(meterCell(f, n.Memory, meterNarrow), " "))
	}
	if waiting > 0 {
		parts = append(parts, f.paint(markWarn)+fmt.Sprintf(" %d degraded", waiting))
	}
	return strings.Join(parts, " · ")
}
