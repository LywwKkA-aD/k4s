package dashboard

import (
	"fmt"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// The degraded states (§4, §10.5): a source that is denied shows — and the
// auth can-i probe, never a 0; one that is absent or not ready shows ○ and
// what to do about it; one that stopped answering keeps its last data,
// dimmed and marked "stale 2m"; a user who may list only one namespace
// sees that namespace's data under a scope note in the banner.

// mainSource is the source a panel's data stands on: when it is stale,
// the whole panel is. Panel 1 marks each line on its own instead.
func mainSource(s dash.Snapshot, p dash.Panel) dash.Avail {
	switch p {
	case dash.PanelNodes:
		return s.Nodes.Avail
	case dash.PanelPods:
		return s.Pods.Avail
	case dash.PanelWorkloads:
		return s.Workloads.Deploy.Avail
	case dash.PanelEvents:
		return s.Events.Avail
	case dash.PanelTopPods:
		return s.TopPods.Avail
	}
	return dash.Avail{}
}

// staleOf reports a panel drawn from the last answer of a source that
// stopped answering, and that source.
func staleOf(s dash.Snapshot, p dash.Panel) (dash.Avail, bool) {
	a := mainSource(s, p)
	return a, a.State == dash.StateStale && !a.Unanswered
}

// joinNotes joins border notes with " · ", skipping empty ones.
func joinNotes(notes ...string) string {
	out := make([]string, 0, len(notes))
	for _, n := range notes {
		if n != "" {
			out = append(out, n)
		}
	}
	return strings.Join(out, " · ")
}

// noScope reports a user who may not list across namespaces and has no
// namespace in the context to fall back to: only :ns can show anything.
func noScope(s dash.Snapshot) bool {
	return s.Pods.Avail.State == dash.StateForbidden && s.Scope.Namespace == "" && s.Scope.Context == ""
}

// bannerNotes lead the verdict when the picture is partial: the cluster
// stopped answering, or the lists are scoped to one namespace, or to none.
func bannerNotes(f frame) []string {
	var out []string
	if !f.staleSince.IsZero() {
		word := "stale since "
		if !f.snap.Ledger[dash.Pods].Answered {
			word = "no answer since "
		}
		out = append(out, f.paint(markOff)+" "+word+f.staleSince.Format("15:04")+" · "+retryNote(f))
	}
	switch s := f.snap; {
	case s.Scope.Fallback:
		out = append(out, f.paint(markFact)+" ns "+clean(s.Scope.Namespace)+" (cluster-wide list forbidden)")
	case noScope(s):
		out = append(out, f.paint(markUnknown)+" cluster-wide lists forbidden · :ns <name>")
	}
	return out
}

// retryNote says when k4s asks the cluster again: at the next tick, or,
// with watch off, only when r is pressed.
func retryNote(f frame) string {
	if f.paused || f.retry <= 0 {
		return "paused, r retries"
	}
	return "retry " + seconds(f.retry)
}

// scopeFacts stand in for chips when there are none but the picture is
// partial: what k4s could see, and that it was fine.
func scopeFacts(f frame) []string {
	s := f.snap
	var out []string
	if readable(s.Pods.Avail) {
		starting := s.Pods.Attention()
		out = append(out, f.paint(markOK)+fmt.Sprintf(" %d pods ok", s.Pods.Total-starting))
		if starting > 0 {
			out = append(out, f.paint(markInfo)+fmt.Sprintf(" %d starting", starting))
		}
	}
	if d := s.Workloads.Deploy; readable(d.Avail) {
		out = append(out, f.paint(markOK)+fmt.Sprintf(" %d/%d deploy ready", d.Ready, d.Total))
	}
	switch s.Nodes.Metrics.State {
	case dash.StateAbsent:
		out = append(out, f.paint(markOff)+" no metrics")
	case dash.StateForbidden:
		out = append(out, f.paint(markUnknown)+" metrics forbidden")
	}
	return out
}

// notDiscovered reports a cluster whose discovery answered without
// metrics.k8s.io: metrics-server is not installed, or not running.
func notDiscovered(s dash.Snapshot) bool {
	u := s.Ledger[dash.MetricsDiscovery]
	return u.Fetched && u.State == dash.StateOK && !s.Caps.Metrics
}

// metricsLines say why panel 2 has no meters or charts: the reason, what
// to do about it, and when k4s looks again. The first line stands alone
// where there is room for only one.
func metricsLines(f frame) []string {
	m := f.snap.Nodes.Metrics
	switch m.State {
	case dash.StateAbsent:
		why := "metrics.k8s.io is not served"
		if notDiscovered(f.snap) {
			why = "metrics.k8s.io is not in discovery"
		}
		out := []string{f.paint(markOff) + " no metrics-server: " + why, "  " + cluster.MetricsHint(f.distro)}
		if notDiscovered(f.snap) {
			out = append(out, "  "+f.muted("re-checked every 60s and on r"))
		}
		return out
	case dash.StateNotReady:
		return []string{f.paint(markOff) + " metrics not ready"}
	case dash.StateForbidden:
		return []string{f.paint(markUnknown) + " metrics forbidden", "  " + f.command(m.CanI)}
	case dash.StateUnknown:
		return []string{f.paint(markFact) + " waiting for metrics"}
	}
	return []string{f.paint(markOff) + " metrics " + m.State.String()}
}

// nodesDenied is panel 2 for a user who may not list nodes: who was
// denied, the probe, where the pods per node still are, and the metrics
// note when there are no metrics either.
func nodesDenied(f frame, h int) []string {
	s := f.snap
	who := ""
	if readable(s.Cluster.You.Avail) && s.Cluster.You.Text != "" {
		who = " for " + clean(s.Cluster.You.Text)
	}
	out := []string{
		" " + f.paint(markUnknown) + " nodes: list forbidden" + who,
		"   " + f.command(s.Nodes.Avail.CanI),
	}
	if where := podsPerNodeNote(s); where != "" {
		out = append(out, "   "+where)
	}
	if readable(s.Nodes.Metrics) || s.Nodes.Metrics.State == dash.StateUnknown {
		return out
	}
	note := metricsLines(f)
	if len(out)+1+len(note) > h {
		return out
	}
	out = append(out, "")
	for _, l := range note {
		out = append(out, " "+l)
	}
	return out
}

// podsPerNodeNote says where the pods per node are while nodes are
// denied: in panel 3 when it shows pods, behind :ns when no list is
// allowed at all, and nowhere while the pods have not answered.
func podsPerNodeNote(s dash.Snapshot) string {
	switch {
	case readable(s.Pods.Avail) && s.Pods.Scope != "":
		return "pods per node (ns " + clean(s.Pods.Scope) + ") still shown in 3 pods"
	case readable(s.Pods.Avail):
		return "pods per node still shown in 3 pods"
	case noScope(s):
		return ":ns <name> to see pods per node"
	}
	return ""
}
