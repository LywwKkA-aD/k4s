package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// kindW is the kind column and the ready bars' label: "deploy", the
// widest kind word, so the table does not shift as rows come and go.
const kindW = 6

// workloadCols is one step of the column drop order: UP-TO-DATE and
// AVAIL, then the namespace in front of the name.
type workloadCols struct {
	counts, ns bool
}

var workloadSteps = []workloadCols{{counts: true, ns: true}, {ns: true}, {}}

// workloadsBody draws panel 4: a ready bar per kind, the jobs and cron
// lines, then the workloads that need a look.
func workloadsBody(f frame, w, h int) panelBody {
	items := workloadItems(f.snap)
	body := panelBody{info: workloadsInfo(f.snap.Workloads)}
	bars := kindLines(f, w)
	short := []string{" " + workloadsShort(f)}
	head := fitHead(h, workloadsNeed(len(items)), slices.Concat(bars, []string{""}), bars, slices.Concat(short, []string{""}), short)
	if len(items) == 0 {
		body.lines = append(slices.Clip(head), allReadyLine(f))
		return body
	}
	table, hidden := workloadTable(f, w, items, h-len(head))
	body.lines = slices.Concat(head, table)
	attention := ""
	if n := needAttention(items); n > 0 {
		attention = fmt.Sprintf("%d need attention", n)
	}
	rows := len(items)
	if len(head) >= kindLineCount {
		rows += 2 // the jobs and cron lines are rows too
	}
	body.footer = f.listFooter(dash.PanelWorkloads, rows, hidden, attention)
	return body
}

// kindLines are the ready bars of deploy, sts and ds, all as wide, then
// the jobs and cron lines, which the cursor can stand on.
func kindLines(f frame, w int) []string {
	kinds := readyKinds(f.snap.Workloads)
	countW := 0
	for _, k := range kinds {
		countW = max(countW, len(readyCount(k.sum)))
	}
	barW := max(w-1-kindW-2-2-countW-1, 1)
	out := make([]string, 0, kindLineCount)
	for _, k := range kinds {
		out = append(out, f.labelled(k.label, kindBar(f, k, barW, countW), ""))
	}
	return append(out, f.labelled("jobs", jobsText(f), jobsKey), f.labelled("cron", cronText(f), cronKey))
}

// labelled is a line of panel 4 under its kind label; key names the row
// it is, "" for a line the cursor skips.
func (f frame) labelled(label, text, key string) string {
	sel := key != "" && f.focus.panel == dash.PanelWorkloads && f.focus.row == key
	return f.listLine(padRight(label, kindW)+"  "+text, sel)
}

func readyCount(k dash.KindSummary) string {
	if !readable(k.Avail) || k.Total == 0 {
		return ""
	}
	return fmt.Sprintf("%d/%d", k.Ready, k.Total)
}

// kindBar is one kind's ready bar: ready █, partly ready ░ and not ready
// at all ▚, the counts after it. A denied kind says so here and nowhere
// else.
func kindBar(f frame, k readyKind, barW, countW int) string {
	switch {
	case !readable(k.sum.Avail):
		return f.unreadable(k.resource, k.sum.Avail)
	case k.sum.Total == 0:
		return f.paint(markOff) + " none"
	}
	waiting, failing := 0, 0
	for _, r := range f.snap.Workloads.Rows {
		switch {
		case r.Key.Kind != k.kind:
		case r.Sev == dash.SevFailing:
			failing++
		case r.Sev == dash.SevWaiting:
			waiting++
		}
	}
	counts := []viz.PhaseCount{
		{Phase: viz.PhaseRunning, Count: k.sum.Ready},
		{Phase: viz.PhasePending, Count: waiting},
		{Phase: viz.PhaseFailing, Count: failing},
		{Phase: viz.PhaseOther, Count: max(k.sum.Total-k.sum.Ready-waiting-failing, 0)},
	}
	return viz.PhaseBar(f.th, barW, counts, 1) + "  " + padLeft(readyCount(k.sum), countW)
}

func jobsText(f frame) string {
	j := f.snap.Workloads.Jobs
	if !readable(j.Avail) {
		return f.unreadable("jobs", j.Avail)
	}
	failed := f.paint(markOK) + " none failed in 1h"
	if j.FailedRecent > 0 {
		failed = f.paint(markErr) + fmt.Sprintf(" %d failed in 1h", j.FailedRecent)
	}
	return failed + fmt.Sprintf(" · %d active", j.Active)
}

func cronText(f frame) string {
	c := f.snap.Workloads.Cron
	switch {
	case !readable(c.Avail):
		return f.unreadable("cronjobs", c.Avail)
	case c.Total == 0:
		return f.paint(markOff) + " no cronjobs"
	}
	parts := []string{strconv.Itoa(c.Total)}
	if c.Suspended > 0 {
		parts = append(parts, f.paint(markOff)+fmt.Sprintf(" %d suspended", c.Suspended))
	}
	if c.HasLast {
		parts = append(parts, "last schedule "+ago(c.LastSchedule))
	}
	return strings.Join(parts, " · ")
}

// allReadyLine stands for the table when no workload needs a look.
func allReadyLine(f frame) string {
	ready, total, unreadable := readyTotals(f.snap.Workloads)
	if unreadable > 0 {
		return " " + f.paint(markOK) + fmt.Sprintf(" the %d k4s can read are ready", ready)
	}
	return " " + f.paint(markOK) + fmt.Sprintf(" all %d ready", total)
}

// workloadTable is the header and the rows that fit in room lines, the
// cursor's row kept on screen; hidden counts the rows left out. A failed
// job or a suspended cronjob says so across the count columns.
func workloadTable(f frame, w int, items []workloadItem, room int) (lines []string, hidden int) {
	avail := w - 4
	var head []string
	var cells [][]string
	var c cols
	for _, step := range workloadSteps {
		head, cells = workloadCells(items, step)
		if c = measureSpans(head, cells, items).atLeast(0, kindW); c.width() <= avail {
			break
		}
	}
	lines = []string{f.muted("   " + c.join(head))}
	rows := make([]row, 0, len(items))
	for _, it := range items {
		rows = append(rows, it.row)
	}
	sel := f.selectedIn(dash.PanelWorkloads, rows)
	room = max(room-1, 0)
	start := window(len(items), room, sel)
	for i := start; i < min(len(items), start+room); i++ {
		text := f.paint(workloadMark(items[i].w)) + " " + c.join(cells[i])
		lines = append(lines, f.listLine(text, i == sel))
	}
	return lines, len(items) - (len(lines) - 1)
}

// measureSpans measures the table without the text a failed job or a
// suspended cronjob prints across the count columns: it sets no width.
func measureSpans(head []string, cells [][]string, items []workloadItem) cols {
	sized := make([][]string, 0, len(cells))
	for i, r := range cells {
		if spans(items[i].w) {
			r = r[:len(r)-1]
		}
		sized = append(sized, r)
	}
	return measure(head, sized, nil, nil)
}

// spans reports a row whose state is a word, not counts.
func spans(r dash.WorkloadRow) bool { return r.Failed || r.Suspended }

func workloadCells(items []workloadItem, c workloadCols) ([]string, [][]string) {
	head := []string{"KIND", "NAME", "READY"}
	if c.ns {
		head[1] = "NAMESPACE/NAME"
	}
	if c.counts {
		head = append(head, "UP-TO-DATE", "AVAIL")
	}
	out := make([][]string, 0, len(items))
	for _, it := range items {
		out = append(out, workloadRowCells(it.w, c))
	}
	return head, out
}

func workloadRowCells(r dash.WorkloadRow, c workloadCols) []string {
	name := clean(r.Key.Name)
	if c.ns {
		ns := markGlyphs[markUnknown]
		if r.Resolved {
			ns = clean(r.Key.Namespace)
		}
		name = ns + "/" + name
	}
	cells := []string{shortKind(r.Key.Kind), name}
	switch {
	case r.Failed:
		return append(cells, "failed "+ago(r.FailedAgo))
	case r.Suspended:
		return append(cells, "suspended")
	}
	cells = append(cells, fmt.Sprintf("%d/%d", r.Ready, r.Desired))
	if !c.counts {
		return cells
	}
	upToDate, available := "", ""
	switch r.Key.Kind {
	case "deployment":
		upToDate, available = strconv.Itoa(r.UpToDate), strconv.Itoa(r.Available)
	case "daemonset":
		available = strconv.Itoa(r.Available)
	}
	return append(cells, upToDate, available)
}
