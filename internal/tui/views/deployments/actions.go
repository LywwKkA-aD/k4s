package deployments

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// kind names a deployment in requests and status messages.
const kind = "deployment"

// onKey runs the view's own bindings; handled is false for keys the table
// navigates with.
func (m Model) onKey(msg tea.KeyPressMsg) (next Model, cmd tea.Cmd, handled bool) {
	switch {
	case key.Matches(msg, m.keys.filter):
		m.filter, cmd = m.filter.Open()
		return m.withRows(), cmd, true
	case key.Matches(msg, m.keys.refresh):
		next, cmd = m.refresh()
	case key.Matches(msg, m.keys.watch):
		m.watch = !m.watch
		m.poll, cmd = m.poll.WithEnabled(m.watch && m.client != nil)
		next = m
	case key.Matches(msg, m.keys.describe):
		next, cmd = m, m.describe()
	case key.Matches(msg, m.keys.logs):
		next, cmd = m.logs()
	case key.Matches(msg, m.keys.forward):
		next, cmd = m, m.forward()
	default:
		return m, nil, false
	}
	return next, cmd, true
}

// refresh is r: a consistent read now, unless a list is already running.
func (m Model) refresh() (Model, tea.Cmd) {
	if m.client == nil {
		return m, statusCmd(status.Warn, "no kubeconfig")
	}
	var ok bool
	if m.poll, ok = m.poll.Begin(); !ok {
		return m, nil
	}
	return m, m.list()
}

func (m Model) describe() tea.Cmd {
	d, why := m.target()
	if why != nil {
		return why
	}
	return request(views.DescribeRequestMsg{Kind: kind, Namespace: d.Namespace, Name: d.Name})
}

func (m Model) forward() tea.Cmd {
	d, why := m.runningTarget()
	if why != nil {
		return why
	}
	return request(views.ForwardRequestMsg{Kind: kind, Namespace: d.Namespace, Name: d.Name})
}

func (m Model) logs() (Model, tea.Cmd) {
	d, why := m.runningTarget()
	if why != nil {
		return m, why
	}
	return m.startLogs(d)
}

// target is the deployment under the cursor, or the status that says why
// there is none.
func (m Model) target() (k8s.Deployment, tea.Cmd) {
	switch {
	case m.client == nil:
		return k8s.Deployment{}, statusCmd(status.Warn, "no kubeconfig")
	case !m.loaded:
		return k8s.Deployment{}, statusCmd(status.Info, "deployments are still loading")
	case m.err != nil:
		return k8s.Deployment{}, statusCmd(status.Warn, "deployments unavailable, press r")
	}
	d, ok := m.selected()
	if !ok {
		return k8s.Deployment{}, statusCmd(status.Info, "no deployment selected")
	}
	return d, nil
}

// runningTarget is target for actions that need a pod: a deployment scaled
// to 0 has none to stream or forward to.
func (m Model) runningTarget() (k8s.Deployment, tea.Cmd) {
	d, why := m.target()
	if why != nil {
		return d, why
	}
	if d.Replicas == 0 {
		return d, statusCmd(status.Warn, kind+" "+d.Name+" has 0 replicas")
	}
	return d, nil
}

func request(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}

func statusCmd(level status.Level, text string) tea.Cmd {
	return request(views.StatusMsg{Level: level, Text: text})
}
