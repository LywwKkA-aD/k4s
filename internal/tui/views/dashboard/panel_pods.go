package dashboard

import (
	"fmt"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// podItem is one row of panel 3 and the pod behind it.
type podItem struct {
	row
	pod dash.PodRow
}

// podItems are panel 3's rows: the problems, most severe and most recent
// first, then up to three pods that restarted in the last day. Two rows
// for same-named pods not yet placed in a namespace get distinct keys.
func podItems(s dash.Snapshot) []podItem {
	all := make([]dash.PodRow, 0, len(s.Pods.Problems)+len(s.Pods.Restarted))
	all = append(append(all, s.Pods.Problems...), s.Pods.Restarted...)
	seen := map[string]int{}
	out := make([]podItem, 0, len(all))
	for _, p := range all {
		key := distinct(seen, p.Key.String())
		r := row{key: key, cmd: p.Cmd, target: p.Key, placed: p.Resolved, container: p.Container}
		out = append(out, podItem{row: r, pod: p})
	}
	return out
}

// podsSummary is panel 3 on one line.
func podsSummary(f frame) string {
	p := f.snap.Pods
	if !readable(p.Avail) {
		return f.unreadable("pods", p.Avail)
	}
	// The rows are most severe first, so the worst is among those kept;
	// the count takes in the rows past the data layer's cap.
	attention, worst := p.Attention(), dash.SevOK
	for _, r := range p.Problems {
		worst = max(worst, r.Sev)
	}
	if attention == 0 {
		return f.paint(markOK) + fmt.Sprintf(" %d pods ok", p.Total)
	}
	return f.paint(markOf(worst)) + fmt.Sprintf(" %d need attention · %d pods", attention, p.Total)
}
