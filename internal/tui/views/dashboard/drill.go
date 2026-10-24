package dashboard

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Enter drills down (M7D.11): a panel opens the k4s list its command
// stands for, in the dashboard's scope; a row of one object describes it;
// a row that stands for a list (the namespaces, the LoadBalancers, the
// node chart) opens that list. Where k4s has no view, Enter says y copies
// the command.

const (
	// kubeSystem is where k3s runs its bundled components.
	kubeSystem = "kube-system"
	// serviceLBKey is panel 6's ServiceLB line.
	serviceLBKey = "k3s/servicelb"
	// nsKey and networkKey are panel 1's namespaces and network lines.
	nsKey      = "ns"
	networkKey = "network"
)

// viewAliases are the command bar spellings the enter hint names a list
// by, as a user would type them.
var viewAliases = map[string]string{
	"namespaces":  "ns",
	"nodes":       "nodes",
	"pods":        "pods",
	"deployments": "deploy",
	"events":      "events",
	"services":    "svc",
	"top":         "top",
}

// panelView is the list panel p stands for as a whole: its command's
// list, in the scope the dashboard lists. The events panel keeps only the
// warnings, as its command does, and top sorts as panel 7 does.
func (m Model) panelView(p dash.Panel) (views.OpenViewMsg, bool) {
	ns := m.mem.snap.Scope.Namespace
	switch p {
	case dash.PanelCluster:
		return views.OpenViewMsg{Name: "namespaces"}, true
	case dash.PanelNodes:
		return views.OpenViewMsg{Name: "nodes"}, true
	case dash.PanelPods:
		return views.OpenViewMsg{Name: "pods", Namespace: ns}, true
	case dash.PanelWorkloads:
		return views.OpenViewMsg{Name: "deployments", Namespace: ns}, true
	case dash.PanelEvents:
		return views.OpenViewMsg{Name: "events", Namespace: ns, State: views.State{Mode: views.ModeWarnings}}, true
	case dash.PanelK3s:
		return views.OpenViewMsg{Name: "deployments", Namespace: kubeSystem}, true
	case dash.PanelTopPods:
		o := views.OpenViewMsg{Name: "top", Namespace: ns}
		if m.mem.topByMemory {
			o.State.Sort = views.SortMemory
		}
		return o, true
	}
	return views.OpenViewMsg{}, false
}

// rowView is the list a row of panel p stands for, false for a row of one
// object or of none.
func (m Model) rowView(p dash.Panel, r row) (views.OpenViewMsg, bool) {
	ns := m.mem.snap.Scope.Namespace
	switch {
	case p == dash.PanelCluster && r.key == nsKey:
		return views.OpenViewMsg{Name: "namespaces"}, true
	case p == dash.PanelCluster && r.key == networkKey, p == dash.PanelK3s && r.key == serviceLBKey:
		return views.OpenViewMsg{Name: "services", Namespace: ns}, true
	case p == dash.PanelNodes && r.key == chartKey:
		return views.OpenViewMsg{Name: "top", Namespace: ns, State: views.State{Mode: views.ModeNodes}}, true
	}
	return views.OpenViewMsg{}, false
}

// onEnter opens what the focus stands for: on a next suggestion what it
// says (logs or describe), on the verdict the panel of the most severe
// finding, on a row its object's describe or its list, on a panel its
// list. Anything else has no k4s view, and y copies its command.
func (m Model) onEnter() (views.View, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	m = m.dropLogs()
	f := m.mem.focus
	if sg, ok := m.suggestionAt(); ok {
		return m, m.runSuggestion(sg)
	}
	if to := m.mem.snap.Verdict.Focus; f.panel == dash.PanelOverview && to != dash.PanelOverview {
		return m.jump(to)
	}
	if r, ok := f.selected(m.rows(f.panel)); ok {
		if o, ok := m.rowView(f.panel, r); ok {
			return m, openCmd(o)
		}
		switch {
		case r.gone:
			return m, statusCmd(status.Info, goneObject(r.target))
		case describable(r):
			return m, describeCmd(r.target)
		}
		return m, statusCmd(status.Info, noViewText)
	}
	if o, ok := m.panelView(f.panel); ok {
		return m, openCmd(o)
	}
	return m, statusCmd(status.Info, noViewText)
}

// enterHint is the enter hint for the footer, "" where Enter only says
// there is no view, or that the panel it would go to is hidden.
func (m Model) enterHint() string {
	f := m.mem.focus
	if sg, ok := m.suggestionAt(); ok {
		return suggestionHint(sg)
	}
	if to := m.mem.snap.Verdict.Focus; f.panel == dash.PanelOverview && to != dash.PanelOverview {
		if !m.reachable(to) {
			return ""
		}
		return "go to " + panelNames[to]
	}
	if r, ok := f.selected(m.rows(f.panel)); ok {
		if o, ok := m.rowView(f.panel, r); ok {
			return viewHint(o)
		}
		if describable(r) {
			return "describe"
		}
		return ""
	}
	if o, ok := m.panelView(f.panel); ok {
		return viewHint(o)
	}
	return ""
}

// onLogs is l: the tail prompt for the pod under the cursor or for the
// pod a next suggestion reads logs of, or the logs of the pods of the
// deployment under the cursor (deploy_logs.go).
func (m Model) onLogs() (views.View, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	f := m.mem.focus
	r, onRow := f.selected(m.rows(f.panel))
	if _, ok := m.suggestionAt(); !ok && onRow && r.deployment() {
		return m.startDeployLogs(r.target)
	}
	m = m.dropLogs()
	if sg, ok := m.suggestionAt(); ok {
		if readsLogs(sg) {
			return m, logsOfSuggestion(sg)
		}
		return m, statusCmd(status.Info, "no logs for this suggestion · y copies the command")
	}
	switch {
	case onRow && r.gone:
		return m, statusCmd(status.Info, goneObject(r.target))
	case onRow && r.pod():
		return m, logsCmd(r.target, r.container)
	}
	return m, statusCmd(status.Info, "no logs here: l reads the logs of a pod or deployment row")
}

// logsHint is the l hint where the cursor is, "" where l opens no logs.
func (m Model) logsHint() string {
	if sg, ok := m.suggestionAt(); ok {
		if readsLogs(sg) {
			return logsWord(sg)
		}
		return ""
	}
	f := m.mem.focus
	if r, ok := f.selected(m.rows(f.panel)); ok && (r.pod() || r.deployment()) {
		return "logs"
	}
	return ""
}

// goneObject is the status for Enter or l on a row whose object is gone:
// "node k3s-agent-2 is gone".
func goneObject(k dash.ObjectKey) string {
	return change{obj: k}.object() + " is gone"
}

// viewHint is the enter hint for opening o: "open :deploy".
func viewHint(o views.OpenViewMsg) string {
	return "open :" + viewAliases[o.Name]
}

// openCmd asks the root for the list o.
func openCmd(o views.OpenViewMsg) tea.Cmd {
	return func() tea.Msg { return o }
}
