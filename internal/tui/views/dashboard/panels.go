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
// draws it, in drawing order. Panels 6 and 7 have none yet: they are
// focused as a whole.
func rowsOf(p dash.Panel, s dash.Snapshot, l layout) []row {
	switch p {
	case dash.PanelCluster:
		items := clusterItems(s)
		out := make([]row, 0, len(items))
		for _, it := range items {
			out = append(out, row{key: it.key, cmd: it.cmd})
		}
		return out
	case dash.PanelNodes:
		return nodeRows(s, l.drawsChart(s))
	case dash.PanelPods:
		items := podItems(s)
		out := make([]row, 0, len(items))
		for _, it := range items {
			out = append(out, it.row)
		}
		return out
	case dash.PanelWorkloads:
		items := workloadItems(s)
		out := make([]row, 0, len(items))
		for _, it := range items {
			out = append(out, it.row)
		}
		return out
	case dash.PanelEvents:
		items := eventItems(s)
		out := make([]row, 0, len(items))
		for _, it := range items {
			out = append(out, it.row)
		}
		return out
	}
	return nil
}

// describeKinds are the kinds k4s has a describe view for.
var describeKinds = []string{"pod", "deployment", "service"}

// describable reports a row Enter opens in k4s's describe view: an object
// of a kind it describes, placed in its namespace.
func describable(r row) bool {
	return r.placed && slices.Contains(describeKinds, r.target.Kind)
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
