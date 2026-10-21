package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// workloadItem is one row of panel 4 and the workload behind it.
type workloadItem struct {
	row
	w dash.WorkloadRow
}

// The jobs and cron lines' row keys.
const (
	jobsKey = "jobs"
	cronKey = "cron"
)

// kindLineCount is the lines above panel 4's table: the deploy, sts and
// ds bars, the jobs line and the cron line.
const kindLineCount = 5

// workloadRows are panel 4's cursor stops: the jobs and cron lines, while
// the panel draws them, then the workloads that need a look.
func workloadRows(s dash.Snapshot, lines bool) []row {
	items := workloadItems(s)
	out := make([]row, 0, len(items)+2)
	if lines {
		out = append(out, row{key: jobsKey, cmd: s.Workloads.Jobs.Cmd}, row{key: cronKey, cmd: s.Workloads.Cron.Cmd})
	}
	for _, it := range items {
		out = append(out, it.row)
	}
	return out
}

// workloadsNeed is how many lines panel 4's list keeps before the lines
// above it fold: a header and up to minListRows rows, or the all-ready
// line.
func workloadsNeed(rows int) int {
	if rows == 0 {
		return 1
	}
	return min(rows, minListRows) + 1
}

// drawsKindLines reports whether panel 4 has room in l for its ready bars
// and the jobs and cron lines, which fold into one line when it has not.
func (l layout) drawsKindLines(s dash.Snapshot) bool {
	pl, ok := l.placeOf(dash.PanelWorkloads)
	if !ok || pl.line {
		return false
	}
	return pl.h-2-kindLineCount >= workloadsNeed(len(s.Workloads.Rows))
}

// workloadItems are panel 4's rows as the data layer orders them: not
// ready, failed jobs and suspended cronjobs, most severe first.
func workloadItems(s dash.Snapshot) []workloadItem {
	seen := map[string]int{}
	out := make([]workloadItem, 0, len(s.Workloads.Rows))
	for _, r := range s.Workloads.Rows {
		key := distinct(seen, r.Key.String())
		out = append(out, workloadItem{row: row{key: key, cmd: r.Cmd, target: r.Key, placed: r.Resolved}, w: r})
	}
	return out
}

// kindWords are the short resource words kubectl users type, as the rows
// print the kinds.
var kindWords = map[string]string{
	"deployment": "deploy", "statefulset": "sts", "daemonset": "ds", "job": "job", "cronjob": "cron",
}

func shortKind(kind string) string {
	if w, ok := kindWords[kind]; ok {
		return w
	}
	return clean(kind)
}

// workloadMark is a row's glyph. A suspended cronjob is off, not minor:
// nothing about it runs.
func workloadMark(r dash.WorkloadRow) mark {
	if r.Suspended {
		return markOff
	}
	return markOf(r.Sev)
}

// readyKind is one ready bar of panel 4: the kind's word, the resource it
// lists and its counts.
type readyKind struct {
	label, kind, resource string
	sum                   dash.KindSummary
}

func readyKinds(w dash.WorkloadsPanel) []readyKind {
	return []readyKind{
		{label: "deploy", kind: "deployment", resource: "deployments", sum: w.Deploy},
		{label: "sts", kind: "statefulset", resource: "statefulsets", sum: w.STS},
		{label: "ds", kind: "daemonset", resource: "daemonsets", sum: w.DS},
	}
}

// readyTotals adds up the readable kinds; unreadable counts the others.
func readyTotals(w dash.WorkloadsPanel) (ready, total, unreadable int) {
	for _, k := range readyKinds(w) {
		if !readable(k.sum.Avail) {
			unreadable++
			continue
		}
		ready, total = ready+k.sum.Ready, total+k.sum.Total
	}
	return ready, total, unreadable
}

// workloadsInfo is the top border's note: "61/65 ready".
func workloadsInfo(w dash.WorkloadsPanel) string {
	ready, total, unreadable := readyTotals(w)
	if unreadable == len(readyKinds(w)) {
		return ""
	}
	return fmt.Sprintf("%d/%d ready", ready, total)
}

// needAttention counts rows at △ or worse.
func needAttention(items []workloadItem) int {
	n := 0
	for _, it := range items {
		if it.w.Sev >= dash.SevWaiting {
			n++
		}
	}
	return n
}

// workloadsSummary is panel 4 on one line: ready out of all deploy, sts
// and ds, failed jobs and suspended cronjobs.
func workloadsSummary(f frame) string {
	w := f.snap.Workloads
	ready, total, unreadable := readyTotals(w)
	worst := dash.SevOK
	for _, r := range w.Rows {
		if r.Key.Kind != "job" && r.Key.Kind != "cronjob" {
			worst = max(worst, r.Sev)
		}
	}
	var parts []string
	if unreadable < len(readyKinds(w)) {
		parts = append(parts, f.paint(markOf(worst))+fmt.Sprintf(" %d/%d ready", ready, total))
	}
	for _, k := range readyKinds(w) {
		if !readable(k.sum.Avail) {
			parts = append(parts, f.unreadable(k.label, k.sum.Avail))
		}
	}
	if n := w.Jobs.FailedRecent; n > 0 {
		parts = append(parts, f.paint(markErr)+" "+plural(n, "job", "jobs")+" failed")
	}
	if n := w.Cron.Suspended; n > 0 {
		parts = append(parts, f.paint(markOff)+" "+plural(n, "cronjob", "cronjobs")+" suspended")
	}
	return strings.Join(parts, " · ")
}

// workloadsShort is the line the ready bars fold into in a short panel:
// "deploy 49/52 · sts 7/8 · ds 5/5 · jobs ▲1 · cron ○1".
func workloadsShort(f frame) string {
	w := f.snap.Workloads
	parts := make([]string, 0, 5)
	for _, k := range readyKinds(w) {
		count := f.paint(markUnknown)
		if readable(k.sum.Avail) {
			count = fmt.Sprintf("%d/%d", k.sum.Ready, k.sum.Total)
		}
		parts = append(parts, k.label+" "+count)
	}
	if n := w.Jobs.FailedRecent; n > 0 {
		parts = append(parts, fmt.Sprintf("jobs %s%d", f.paint(markErr), n))
	}
	if n := w.Cron.Suspended; n > 0 {
		parts = append(parts, fmt.Sprintf("cron %s%d", f.paint(markOff), n))
	}
	return strings.Join(parts, " · ")
}
