package dashboard

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

const (
	// notOKStripW is the not-ok strip: 30 one-minute samples.
	notOKStripW = 30
	// minListRows is how short a list gets before the lines above it go.
	minListRows = 3
)

// podCols is one step of the column drop order: NODE, then AGE, then the
// "(2m ago)" suffix, then NAMESPACE merges into the name.
type podCols struct {
	node, age, ago, ns bool
}

var podSteps = []podCols{
	{node: true, age: true, ago: true, ns: true},
	{age: true, ago: true, ns: true},
	{ago: true, ns: true},
	{ns: true},
	{},
}

// podsBody draws panel 3: the phase bar and legend, the not-ok strip, and
// the rows that need a look.
func podsBody(f frame, w, h int) panelBody {
	p := f.snap.Pods
	body := panelBody{info: podsInfo(f.snap)}
	switch {
	case p.Avail.State == dash.StateForbidden:
		denied := " " + f.paint(markUnknown) + " pods forbidden"
		if noScope(f.snap) {
			denied += " · :ns <name> to switch"
		}
		body.lines = []string{denied, "   " + f.command(p.Avail.CanI)}
		return body
	case !readable(p.Avail):
		body.lines = []string{" " + f.unreadable("pods", p.Avail)}
		return body
	}
	items := podItems(f.snap)
	head := podsHead(f, w, items, h)
	room := h - len(head)
	if len(items) == 0 {
		body.lines = append(head, emptyPods(f))
		return body
	}
	table, hidden := podTable(f, w, items, room)
	body.lines = append(head, table...)
	body.footer = podsFooter(f, items, hidden)
	return body
}

// podsHead is what stands above the rows, losing lines from the bottom up
// (the gap, the strip, the legend) while the rows would get fewer than
// minListRows. Only the wide layout has room for the ≈ strip; the narrow
// ones fold the legend into its counts beside the bar.
func podsHead(f frame, w int, items []podItem, h int) []string {
	need := min(len(items), minListRows) + 1 // rows plus their header
	if len(items) == 0 {
		need = 1
	}
	counts := phaseCounts(f.snap.Pods)
	if f.narrow() {
		tally := phaseTally(f, counts)
		bar := viz.PhaseBar(f.th, max(w-2-2-ansi.StringWidth(tally)-1, 0), counts, 1)
		return fitHead(h, need, []string{" " + bar + "  " + tally})
	}
	total := "  " + strconv.Itoa(f.snap.Pods.Total)
	bar := " " + viz.PhaseBar(f.th, max(w-2-len(total), 0), counts, 1) + total
	legend := " " + viz.PhaseLegend(f.th, w-2, counts)
	if f.kind != kindWide {
		return fitHead(h, need, []string{bar, legend}, []string{bar})
	}
	strip := " " + notOKLine(f, w-2)
	return fitHead(h, need, []string{bar, legend, strip, ""}, []string{bar, legend, strip}, []string{bar, legend}, []string{bar})
}

// phaseTally is the legend as counts alone, "█171 ▓7 ░2 ▚4 ·1": one per
// phase with pods, in bar order.
func phaseTally(f frame, counts []viz.PhaseCount) string {
	totals := map[viz.Phase]int{}
	for _, c := range counts {
		totals[c.Phase] += c.Count
	}
	parts := make([]string, 0, len(totals))
	for _, p := range []viz.Phase{viz.PhaseRunning, viz.PhaseCompleted, viz.PhasePending, viz.PhaseFailing, viz.PhaseOther} {
		if totals[p] == 0 {
			continue
		}
		st := f.th.Muted
		switch p {
		case viz.PhaseRunning:
			st = f.th.OK
		case viz.PhasePending:
			st = f.th.Warn
		case viz.PhaseFailing:
			st = f.th.Err
		}
		parts = append(parts, st.Render(p.Glyph())+strconv.Itoa(totals[p]))
	}
	return strings.Join(parts, " ")
}

// phaseCounts feeds the phase bar and its legend: one failing entry per
// reason, so the legend reads "▚ CrashLoopBackOff 3 · ImagePullBackOff 1".
func phaseCounts(p dash.PodsPanel) []viz.PhaseCount {
	out := []viz.PhaseCount{
		{Phase: viz.PhaseRunning, Count: p.Phases.Running},
		{Phase: viz.PhaseCompleted, Count: p.Phases.Completed},
		{Phase: viz.PhasePending, Count: p.Phases.Waiting},
	}
	failing := 0
	for _, rc := range p.Failing {
		out = append(out, viz.PhaseCount{Phase: viz.PhaseFailing, Count: rc.Count, Label: clean(rc.Reason)})
		failing += rc.Count
	}
	if rest := p.Phases.Failing - failing; rest > 0 {
		out = append(out, viz.PhaseCount{Phase: viz.PhaseFailing, Count: rest})
	}
	return append(out,
		viz.PhaseCount{Phase: viz.PhaseOther, Count: p.Phases.Evicted, Label: "Evicted"},
		viz.PhaseCount{Phase: viz.PhaseOther, Count: p.Phases.Other, Label: "Other"},
	)
}

// notOKLine is the ≈ strip of pods that are not ok, one point a minute,
// with the count now and the pods per node.
func notOKLine(f frame, w int) string {
	strip := viz.Spark(f.th, notOKStripW, f.hist.notOK.values(), 0)
	text := f.ds.Approx.Render("≈"+minutes(notOKStripW*notOKStep.Seconds())+"m") + " not ok " + strip +
		fmt.Sprintf("  %d now", f.snap.Pods.NotOK)
	if per := perNode(f.snap.Pods.PerNode); per != "" {
		text += "   " + f.muted("per node") + "  " + per
	}
	return cutText(text, w)
}

func perNode(counts []dash.NodeCount) string {
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, clean(c.Node)+" "+strconv.Itoa(c.Pods))
	}
	return strings.Join(parts, " · ")
}

func emptyPods(f frame) string {
	p := f.snap.Pods
	if p.Total == 0 {
		where := "in any namespace"
		if p.Scope != "" {
			where = "in " + clean(p.Scope)
		}
		return " " + f.paint(markOff) + " no pods " + where
	}
	return " " + f.paint(markOK) + " nothing needs attention"
}

// podTable is the header and the rows that fit in room lines, the
// cursor's row kept on screen; hidden counts the rows left out.
func podTable(f frame, w int, items []podItem, room int) (lines []string, hidden int) {
	avail := w - 4
	var head []string
	var cells [][]string
	var c cols
	for _, step := range podSteps {
		head, cells = podCells(items, step)
		if c = measure(head, cells, nil, nil); c.width() <= avail {
			break
		}
	}
	lines = []string{f.muted("   " + c.join(head))}
	sel := -1
	for i, it := range items {
		if f.focus.panel == dash.PanelPods && f.focus.row == it.key {
			sel = i
		}
	}
	rows := max(room-1, 0)
	start := window(len(items), rows, sel)
	for i := start; i < min(len(items), start+rows); i++ {
		text := f.paint(markOf(items[i].pod.Sev)) + " " + c.join(cells[i])
		lines = append(lines, f.listLine(text, i == sel))
	}
	return lines, len(items) - (len(lines) - 1)
}

func podCells(items []podItem, cols podCols) ([]string, [][]string) {
	head := []string{"NAME", "READY", "STATUS", "RESTARTS"}
	if cols.ns {
		head = append([]string{"NAMESPACE"}, head...)
	}
	if cols.age {
		head = append(head, "AGE")
	}
	if cols.node {
		head = append(head, "NODE")
	}
	out := make([][]string, 0, len(items))
	for _, it := range items {
		out = append(out, podRowCells(it.pod, cols))
	}
	return head, out
}

func podRowCells(p dash.PodRow, cols podCols) []string {
	ns := markGlyphs[markUnknown]
	if p.Resolved {
		ns = clean(p.Key.Namespace)
	}
	name := clean(p.Key.Name)
	var cells []string
	if cols.ns {
		cells = append(cells, ns)
	} else if p.Resolved {
		name = ns + "/" + name
	}
	ready := markGlyphs[markUnknown]
	if p.ReadyKnown {
		ready = fmt.Sprintf("%d/%d", p.Ready, p.ReadyOf)
	}
	restarts := strconv.Itoa(p.Restarts)
	if cols.ago && p.HasRestart && p.Restarts > 0 {
		restarts += " (" + ago(p.LastRestart) + " ago)"
	}
	cells = append(cells, name, ready, clean(p.Status), restarts)
	if cols.age {
		cells = append(cells, ago(p.Age))
	}
	if cols.node {
		node := "<none>"
		if p.Node != "" {
			node = clean(p.Node)
		}
		cells = append(cells, node)
	}
	return cells
}

// podsInfo is the top border's note: "185 · 33 ns", or "41 · ns shop".
func podsInfo(s dash.Snapshot) string {
	p := s.Pods
	if !readable(p.Avail) {
		return ""
	}
	if p.Scope != "" {
		return fmt.Sprintf("%d · ns %s", p.Total, clean(p.Scope))
	}
	if readable(s.Cluster.Namespaces.Avail) {
		return fmt.Sprintf("%d · %d ns", p.Total, s.Cluster.Namespaces.Count)
	}
	return strconv.Itoa(p.Total)
}

// podsFooter is the bottom border's note: the cursor's place, rows left
// out, and how many need attention or restarted.
func podsFooter(f frame, items []podItem, hidden int) string {
	var parts []string
	if f.focus.panel == dash.PanelPods && f.focus.row != "" {
		parts = append(parts, fmt.Sprintf("%d/%d", f.focus.index+1, len(items)))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("+%d", hidden))
	}
	attention := 0
	for _, it := range items {
		if it.pod.Sev >= dash.SevWaiting {
			attention++
		}
	}
	if attention > 0 {
		parts = append(parts, fmt.Sprintf("%d need attention", attention))
	}
	if n := len(f.snap.Pods.Restarted); n > 0 {
		parts = append(parts, fmt.Sprintf("%d restarting", n))
	}
	return strings.Join(parts, " · ")
}
