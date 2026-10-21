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
