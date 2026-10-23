package dashboard

import (
	"slices"
	"strconv"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// panelNames name the panels after their number; the number never changes
// with the terminal size.
var panelNames = [...]string{
	dash.PanelOverview:  "overview",
	dash.PanelCluster:   "cluster",
	dash.PanelNodes:     "nodes",
	dash.PanelPods:      "pods",
	dash.PanelWorkloads: "workloads",
	dash.PanelEvents:    "events",
	dash.PanelK3s:       "k3s",
	dash.PanelTopPods:   "top pods",
}

// panelLabel is the title a panel's box carries: "3 pods".
func panelLabel(p dash.Panel) string {
	return strconv.Itoa(int(p)) + " " + panelNames[p]
}

// panelCommand is panelCmd with panel 7's --sort-by following s.
func (m Model) panelCommand(p dash.Panel) dash.Cmd {
	if p == dash.PanelTopPods && m.mem.topByMemory {
		return m.mem.snap.TopPods.CmdMemory
	}
	return panelCmd(p, m.mem.snap)
}

// panelCmd is the command of a panel as a whole; the banner's is the
// verdict's.
func panelCmd(p dash.Panel, s dash.Snapshot) dash.Cmd {
	switch p {
	case dash.PanelCluster:
		return s.Cluster.Cmd
	case dash.PanelNodes:
		return s.Nodes.Cmd
	case dash.PanelPods:
		return s.Pods.Cmd
	case dash.PanelWorkloads:
		return s.Workloads.Cmd
	case dash.PanelEvents:
		return s.Events.Cmd
	case dash.PanelK3s:
		return s.K3s.Cmd
	case dash.PanelTopPods:
		return s.TopPods.CmdCPU
	}
	return s.Verdict.Cmd
}

// rowsOf lists the rows the cursor can stand on in panel p as layout l
// draws it, in drawing order; none where p is a summary line.
func rowsOf(p dash.Panel, mem Memory, l layout) []row {
	pl, ok := l.placeOf(p)
	if ok && pl.line {
		return nil
	}
	s := mem.snap
	switch p {
	case dash.PanelOverview:
		return nextRows(s, l)
	case dash.PanelCluster:
		return rowsFrom(clusterItems(s), func(it clusterItem) row { return row{key: it.key, cmd: it.cmd} })
	case dash.PanelNodes:
		return nodeRows(s, l.drawsChart(s))
	case dash.PanelPods:
		return rowsFrom(podItems(s), func(it podItem) row { return it.row })
	case dash.PanelWorkloads:
		return workloadRows(s, l.drawsKindLines(s))
	case dash.PanelEvents:
		out := rowsFrom(eventItems(s), func(it eventItem) row { return it.row })
		if !ok {
			return out
		}
		return append(out, ringRows(mem.ring[:ringShown(len(mem.ring), l.kind, l.zoom, pl.h-2)])...)
	case dash.PanelK3s:
		return rowsFrom(k3sItems(s), func(it k3sItem) row { return it.row })
	case dash.PanelTopPods:
		return topRows(topItems(s, mem.topByMemory))
	}
	return nil
}

// rowsFrom is the row of each item, in order.
func rowsFrom[T any](items []T, rowOf func(T) row) []row {
	out := make([]row, 0, len(items))
	for _, it := range items {
		out = append(out, rowOf(it))
	}
	return out
}

// describeKinds are the kinds of the dashboard's rows that k4s's describe
// view shows: it writes pods, deployments and services itself and runs
// kubectl describe for the others.
var describeKinds = []string{"pod", "deployment", "service", "node", "statefulset", "daemonset", "job", "cronjob"}

// describable reports a row Enter opens in k4s's describe view: an object
// of a kind it describes, placed in its namespace (a node has none).
func describable(r row) bool {
	return r.placed && !r.gone && slices.Contains(describeKinds, r.target.Kind)
}

// toKubectl turns a data-layer command into the footer's, its arguments
// cleaned of anything a terminal would act on.
func toKubectl(c dash.Cmd) kubectl.Cmd {
	if c.IsZero() {
		return kubectl.Cmd{}
	}
	out := kubectl.New(cleanArgs(c.Args)...)
	out.Approx = c.Approx
	return out
}
