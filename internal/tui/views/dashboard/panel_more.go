package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// Panels 6 and 7 are drawn as their one-line summary until their own
// renderers land with M7D.9. They are focused as a whole and carry their
// panel command.

// summaryBody is a panel drawn as its summary line.
func summaryBody(f frame, p dash.Panel) panelBody {
	return panelBody{lines: []string{" " + summaryOf(f, p)}}
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
