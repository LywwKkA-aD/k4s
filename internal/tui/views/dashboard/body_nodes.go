package dashboard

import (
	"fmt"
	"math"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const (
	// stripW is a node's ≈ strip: 18 metrics samples, about 5 minutes.
	stripW = 18
	// meterWide and meterValue are the 12-cell meter and the value alone,
	// around meterNarrow (see viz.Meter).
	meterWide  = 18
	meterValue = 5
	// chartRows is the braille chart's height when there is room.
	chartRows = 2
)

// nodeCols is one step of the column drop order: ROLES first, then the
// per-node strips, AGE and PODS, then the meters narrow to 6 cells and to
// the value alone.
type nodeCols struct {
	roles, strips, age, pods bool
	meter                    int
}

var nodeSteps = []nodeCols{
	{roles: true, strips: true, age: true, pods: true, meter: meterWide},
	{strips: true, age: true, pods: true, meter: meterWide},
	{age: true, pods: true, meter: meterWide},
	{pods: true, meter: meterWide},
	{meter: meterWide},
	{meter: meterNarrow},
	{meter: meterValue},
}

// nodesBody draws panel 2: a table of nodes over the cluster history.
func nodesBody(f frame, w, h int) panelBody {
	n := f.snap.Nodes
	body := panelBody{info: fmt.Sprintf("%d/%d Ready", n.Ready, n.Total), footer: kubeletNote(n.Rows)}
	switch {
	case n.Avail.State == dash.StateForbidden:
		body.info = markGlyphs[markUnknown]
		body.lines = []string{
			" " + f.paint(markUnknown) + " nodes forbidden",
			"   " + toKubectl(n.Avail.CanI).String(),
			"   pods per node are still shown in 3 pods",
		}
		return body
	case !readable(n.Avail):
		body.info = ""
		body.lines = []string{" " + f.unreadable("nodes", n.Avail)}
		return body
	case len(n.Rows) == 0:
		body.lines = []string{" " + f.paint(markOff) + " no nodes registered"}
		return body
	}
	tail := chartLines(f, w, chartSize(f.snap, len(n.Rows), h))
	room := h - 1 - len(tail)
	table, hidden := nodeTable(f, w, room)
	body.lines = append(table, tail...)
	if hidden > 0 {
		body.footer = fmt.Sprintf("+%d · %s", hidden, body.footer)
	}
	return body
}

// chartSize is how many lines the history takes under the table: the full
// chart while the table keeps its rows (at least three), then a one-row
// chart, then none. Without metrics it is the one-line note.
func chartSize(s dash.Snapshot, nodes, h int) int {
	if !hasMetrics(s) {
		if h-1-2 >= min(nodes, 3) {
			return 2
		}
		return 0
	}
	for _, size := range []int{3 + chartRows, 3 + 1, 2 + 1} {
		if h-1-size >= min(nodes, 3) {
			return size
		}
	}
	return 0
}

// nodeTable is the header and the node rows that fit in room lines, the
// cursor's row kept on screen; hidden counts the rows left out.
func nodeTable(f frame, w, room int) (lines []string, hidden int) {
	rows := f.snap.Nodes.Rows
	avail := w - 4 // cursor, glyph, space, right margin
	var head []string
	var cells [][]string
	var c cols
	for _, step := range nodeSteps {
		head, cells = nodeCells(f, step)
		if c = measure(head, cells, isPodsHead, isMeterHead); c.width() <= avail {
			break
		}
	}
	lines = append(lines, f.muted("   "+c.join(head)))
	sel := -1
	for i, r := range rows {
		if f.focus.panel == dash.PanelNodes && f.focus.row == r.Key.String() {
			sel = i
		}
	}
	room = max(room, 0)
	start := window(len(rows), room, sel)
	for i := start; i < min(len(rows), start+room); i++ {
		text := f.paint(markOf(rows[i].Sev)) + " " + c.join(cells[i])
		lines = append(lines, f.listLine(text, i == sel))
	}
	return lines, len(rows) - (len(lines) - 1)
}

func isPodsHead(h string) bool { return h == "PODS" }

func isMeterHead(h string) bool { return h == "CPU" || h == "MEMORY" }

// nodeCells builds the header and every row's cells for one column step.
func nodeCells(f frame, cols nodeCols) ([]string, [][]string) {
	metrics := hasMetrics(f.snap)
	head := []string{"NODE"}
	if cols.roles {
		head = append(head, "ROLES")
	}
	head = append(head, "STATUS")
	if metrics {
		head = append(head, "CPU")
		if cols.strips {
			head = append(head, stripLabel())
		}
		head = append(head, "MEMORY")
		if cols.strips {
			head = append(head, stripLabel())
		}
	}
	if cols.pods {
		head = append(head, "PODS")
	}
	if cols.age {
		head = append(head, "AGE")
	}
	out := make([][]string, 0, len(f.snap.Nodes.Rows))
	for _, n := range f.snap.Nodes.Rows {
		out = append(out, nodeRowCells(f, n, cols, metrics))
	}
	return head, out
}

func nodeRowCells(f frame, n dash.NodeRow, cols nodeCols, metrics bool) []string {
	cells := []string{clean(n.Key.Name)}
	if cols.roles {
		cells = append(cells, rolesText(n.Roles))
	}
	cells = append(cells, statusWord(n))
	if metrics {
		nh := f.hist.nodes[n.Key.Name]
		cells = append(cells, meterCell(f, n.CPU, cols.meter))
		if cols.strips {
			cells = append(cells, viz.Spark(f.th, stripW, nh.cpu.values(), 100))
		}
		cells = append(cells, meterCell(f, n.Memory, cols.meter))
		if cols.strips {
			cells = append(cells, viz.Spark(f.th, stripW, nh.mem.values(), 100))
		}
	}
	if cols.pods {
		pods := markGlyphs[markUnknown]
		if n.PodsKnown {
			pods = fmt.Sprintf("%d/%d", n.Pods, n.PodsCap)
		}
		cells = append(cells, pods)
	}
	if cols.age {
		cells = append(cells, ago(n.Age))
	}
	return cells
}

// stripLabel heads a strip column with the time it spans: "≈5m".
func stripLabel() string {
	return "≈" + minutes(stripW*metricsStep.Seconds()) + "m"
}

func minutes(seconds float64) string {
	return fmt.Sprintf("%d", int(math.Round(seconds/60)))
}

func rolesText(roles []string) string {
	if len(roles) == 0 {
		return "<none>"
	}
	return clean(strings.Join(roles, ","))
}

// statusWord is the node's state: a condition other than Ready replaces
// the word, and so does a pressure or a cordon.
func statusWord(n dash.NodeRow) string {
	switch {
	case n.Condition != "Ready":
		return clean(n.Condition)
	case len(n.Pressures) > 0:
		return clean(n.Pressures[0])
	case n.Unschedulable:
		return "SchedulingDisabled"
	}
	return "Ready"
}

// kubeletNote is the bottom border's kubelet version, or how many differ.
func kubeletNote(rows []dash.NodeRow) string {
	seen := map[string]bool{}
	first := ""
	for _, r := range rows {
		if r.Kubelet == "" {
			continue
		}
		if first == "" {
			first = r.Kubelet
		}
		seen[r.Kubelet] = true
	}
	switch len(seen) {
	case 0:
		return ""
	case 1:
		return "kubelet " + clean(first)
	}
	return fmt.Sprintf("%d kubelet versions", len(seen))
}
