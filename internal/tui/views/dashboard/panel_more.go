package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Panels 4 to 7 are drawn as their one-line summary until their own
// renderers land: workloads and events with M7D.8, k3s and top pods with
// M7D.9. They are focused as a whole and carry their panel command.

// summaryBody is a panel drawn as its summary line.
func summaryBody(f frame, p dash.Panel) panelBody {
	return panelBody{lines: []string{" " + summaryOf(f, p)}}
}

// workloadsSummary is panel 4 on one line: ready out of all deploy, sts
// and ds, failed jobs and suspended cronjobs.
func workloadsSummary(f frame) string {
	w := f.snap.Workloads
	worst, ready, total := dash.SevOK, 0, 0
	var notes []string
	for _, k := range []struct {
		name string
		s    dash.KindSummary
	}{{"deploy", w.Deploy}, {"sts", w.STS}, {"ds", w.DS}} {
		if !readable(k.s.Avail) {
			notes = append(notes, f.unreadable(k.name, k.s.Avail))
			continue
		}
		ready, total = ready+k.s.Ready, total+k.s.Total
	}
	for _, r := range w.Rows {
		if r.Key.Kind != "job" && r.Key.Kind != "cronjob" {
			worst = max(worst, r.Sev)
		}
	}
	var parts []string
	if len(notes) < 3 {
		parts = append(parts, f.paint(markOf(worst))+fmt.Sprintf(" %d/%d ready", ready, total))
	}
	parts = append(parts, notes...)
	if n := w.Jobs.FailedRecent; n > 0 {
		parts = append(parts, f.paint(markErr)+" "+plural(n, "job", "jobs")+" failed")
	}
	if n := w.Cron.Suspended; n > 0 {
		parts = append(parts, f.paint(markOff)+" "+plural(n, "cronjob", "cronjobs")+" suspended")
	}
	return strings.Join(parts, " · ")
}

// eventsSummary is panel 5 on one line: the warnings of the last hour and
// their most frequent reasons.
func eventsSummary(f frame) string {
	e := f.snap.Events
	switch {
	case !readable(e.Avail):
		return f.unreadable("events", e.Avail)
	case e.Total == 0:
		return f.paint(markOK) + " no warnings in the last hour"
	}
	m := markFact
	if e.Counted > 0 {
		m = markWarn
	}
	count := plural(e.Total, "warning", "warnings")
	if e.Truncated {
		count = "≥" + count
	}
	parts := []string{f.paint(m) + " " + count + " in 1h"}
	seen := map[string]bool{}
	for _, r := range e.Rows {
		if len(seen) == 3 {
			break
		}
		if reason := clean(r.Reason); !seen[reason] {
			seen[reason] = true
			parts = append(parts, reason)
		}
	}
	return strings.Join(parts, " · ")
}

// k3sSummary is panel 6 on one line: bundled components, ServiceLB and
// the etcd snapshot.
func k3sSummary(f frame) string {
	k := f.snap.K3s
	if !k.Shown {
		return f.paint(markOff) + " " + clean(k.Hidden)
	}
	ok, worst := 0, dash.SevOK
	for _, c := range k.Components {
		if c.Present && c.Sev == dash.SevOK {
			ok++
		}
		worst = max(worst, c.Sev)
	}
	parts := []string{f.paint(markOf(worst)) + fmt.Sprintf(" %d/%d components", ok, len(k.Components))}
	if k.ServiceLB.State == dash.StateAbsent {
		parts = append(parts, f.paint(markOff)+" servicelb off")
	}
	if k.Etcd.Shown && k.Etcd.Known {
		word := "etcd snapshot"
		if k.Etcd.Reconciled {
			word = "snapshots reconciled"
		}
		parts = append(parts, f.paint(markFact)+" "+word+" "+ago(k.Etcd.Ago)+" ago")
	}
	return strings.Join(parts, " · ")
}

// topPodsSummary is panel 7 on one line: the three busiest pods by CPU.
func topPodsSummary(f frame) string {
	t := f.snap.TopPods
	switch {
	case !readable(t.Avail):
		return f.unreadable("pod metrics", t.Avail)
	case len(t.ByCPU) == 0:
		return f.paint(markOff) + " no pod reports metrics"
	}
	parts := make([]string, 0, 3)
	for _, u := range t.ByCPU[:min(3, len(t.ByCPU))] {
		parts = append(parts, f.paint(markFact)+" "+clean(u.Key.Name)+" "+dash.FormatCPU(u.CPUMilli))
	}
	return strings.Join(parts, " · ")
}
