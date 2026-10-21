package dashboard

import (
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Panel 7 is kubectl top pods: the busiest pods by CPU or, after s, by
// memory, in kubectl's units, with a bar relative to the busiest. It is
// on screen only with metrics-server, and pod metrics are polled only
// while it is.

const (
	// topBarCells is the relative bar beside the sorted measure.
	topBarCells = 8
	// minTopNameW is the narrowest the name column gets before the line
	// is cut at the box edge instead.
	minTopNameW = 12
)

// partials are the eighth blocks that end a bar, by eighths left over.
var partials = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// topItem is one row of panel 7.
type topItem struct {
	row
	u dash.PodUsage
}

// topUsage is the panel's pods in the order shown.
func topUsage(t dash.TopPodsPanel, byMemory bool) []dash.PodUsage {
	if byMemory {
		return t.ByMemory
	}
	return t.ByCPU
}

// topItems are panel 7's rows, keyed by pod.
func topItems(s dash.Snapshot, byMemory bool) []topItem {
	pods := topUsage(s.TopPods, byMemory)
	out := make([]topItem, 0, len(pods))
	for _, u := range pods {
		r := row{key: "top/" + u.Key.String(), cmd: u.Cmd, target: u.Key, placed: u.Key.Namespace != ""}
		out = append(out, topItem{row: r, u: u})
	}
	return out
}

// topBody draws panel 7: the header, a row per pod and the footnote.
func topBody(f frame, w, h int) panelBody {
	t := f.snap.TopPods
	body := panelBody{info: "by cpu"}
	if f.topByMemory {
		body.info = "by memory"
	}
	switch {
	case t.Avail.State == dash.StateForbidden:
		body.lines = []string{" " + f.paint(markUnknown) + " pod metrics forbidden", "   " + f.command(t.Avail.CanI)}
		return body
	case !readable(t.Avail):
		body.lines = []string{" " + f.unreadable("pod metrics", t.Avail)}
		return body
	}
	items := topItems(f.snap, f.topByMemory)
	if len(items) == 0 {
		body.lines = []string{" " + f.paint(markOff) + " no pod reports metrics"}
		return body
	}
	table, hidden := topTable(f, w, items, h)
	body.lines = table
	body.footer = f.listFooter(dash.PanelTopPods, len(items), hidden)
	return body
}

// topTable is the header, the rows that fit in h lines with the cursor's
// on screen, and the footnote when there is a line to spare.
func topTable(f frame, w int, items []topItem, h int) (lines []string, hidden int) {
	head := []string{"NAMESPACE/POD", "CPU(cores)", "MEMORY"}
	cpuTop, memTop := int64(1), int64(1)
	for _, it := range items {
		cpuTop, memTop = max(cpuTop, it.u.CPUMilli), max(memTop, it.u.MemBytes)
	}
	cells := make([][]string, 0, len(items))
	for _, it := range items {
		cpu, mem := dash.FormatCPU(it.u.CPUMilli), dash.FormatMemory(it.u.MemBytes)
		cells = append(cells, []string{clean(it.u.Key.Namespace + "/" + it.u.Key.Name), cpu, mem})
	}
	sorted := 1
	if f.topByMemory {
		sorted = 2
	}
	valueW := 0
	for _, cell := range cells {
		valueW = max(valueW, len(cell[sorted]))
	}
	c := measure(head, cells, func(h string) bool { return h != head[0] }, nil)
	c = c.atLeast(sorted, topBarCells+2+valueW)
	nameW := max(w-4-(c.width()-c.widths[0]), minTopNameW)
	c.widths = append([]int{min(c.widths[0], nameW)}, c.widths[1:]...)

	lines = []string{f.muted("   " + c.join(head))}
	rows := topRows(items)
	sel := f.selectedIn(dash.PanelTopPods, rows)
	room := max(h-1, 0)
	note := f.muted(topNote(f))
	if room > len(items) {
		room = len(items)
	} else if room > 0 {
		note = ""
	}
	start := window(len(items), room, sel)
	for i := start; i < min(len(items), start+room); i++ {
		row := []string{cutText(cells[i][0], c.widths[0]), cells[i][1], cells[i][2]}
		top, v := cpuTop, items[i].u.CPUMilli
		if f.topByMemory {
			top, v = memTop, items[i].u.MemBytes
		}
		row[sorted] = f.ds.Fact.Render(relBar(float64(v)/float64(top)*f.prog, topBarCells)) + "  " + padLeft(row[sorted], valueW)
		text := f.paint(markFact) + " " + c.join(row)
		lines = append(lines, f.listLine(text, i == sel))
	}
	hidden = len(items) - (len(lines) - 1)
	if note != "" {
		lines = append(lines, "   "+note)
	}
	return lines, hidden
}

func topRows(items []topItem) []row {
	out := make([]row, 0, len(items))
	for _, it := range items {
		out = append(out, it.row)
	}
	return out
}

// topNote is the footnote: what the bar means, how many pods report, and
// what s does next.
func topNote(f frame) string {
	next := "s memory"
	if f.topByMemory {
		next = "s cpu"
	}
	return fmt.Sprintf("bar relative to the top pod · %s report · %s", plural(f.snap.TopPods.Reporting, "pod", "pods"), next)
}

// relBar draws frac (0 to 1) as cells of eighth blocks on a · track.
func relBar(frac float64, cells int) string {
	frac = math.Min(math.Max(frac, 0), 1)
	if math.IsNaN(frac) {
		frac = 0
	}
	eighths := int(math.Round(frac * float64(cells*8)))
	full, part := eighths/8, eighths%8
	used := full
	if part > 0 {
		used++
	}
	return strings.Repeat("█", full) + partials[part] + strings.Repeat("·", cells-used)
}

// replicaSuffix is the "-<hash>-<id>" a Deployment's pods carry.
var replicaSuffix = regexp.MustCompile(`-[a-z0-9]{6,10}-[a-z0-9]{5}$`)

// workloadName is a pod's name without its ReplicaSet suffix, as the
// accordion line names the busiest pods: "payments-api".
func workloadName(pod string) string {
	return replicaSuffix.ReplaceAllString(pod, "")
}

// topPodsSummary is panel 7 on one line: the three busiest pods.
func topPodsSummary(f frame) string {
	t := f.snap.TopPods
	switch {
	case !readable(t.Avail):
		return f.unreadable("pod metrics", t.Avail)
	case len(topUsage(t, f.topByMemory)) == 0:
		return f.paint(markOff) + " no pod reports metrics"
	}
	pods := topUsage(t, f.topByMemory)
	parts := make([]string, 0, 3)
	for _, u := range pods[:min(3, len(pods))] {
		v := dash.FormatCPU(u.CPUMilli)
		if f.topByMemory {
			v = dash.FormatMemory(u.MemBytes)
		}
		parts = append(parts, clean(workloadName(u.Key.Name))+" "+v)
	}
	return f.paint(markFact) + " " + strings.Join(parts, " · ")
}
