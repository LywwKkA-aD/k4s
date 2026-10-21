package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// newDashboard builds the dashboard in the active namespace, with what the
// last dashboard learned, the terminal's focus as last reported and the
// session the footer decorates with, so commands printed in its body paste
// as the footer's do.
func (m Model) newDashboard() views.View {
	return dashboard.New(m.client, m.loadErr,
		dashboard.WithNamespace(m.namespace),
		dashboard.WithMemory(m.dashMem),
		dashboard.WithBlurred(m.blurred),
		dashboard.WithSession(m.kubectlSession))
}

// keepDashboard takes v's memory when v is a dashboard about to be
// replaced, so the next one shows its data and history at once. Unlike
// history entries it survives going home; :ctx drops it.
func (m Model) keepDashboard(v views.View) Model {
	if d, ok := v.(dashboard.Model); ok {
		m.dashMem = d.Memory()
	}
	return m
}

// onTerminalFocus remembers whether the terminal has focus, for dashboards
// built later, and hands the message to the active view: the dashboard
// polls slower while blurred.
func (m Model) onTerminalFocus(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, m.blurred = msg.(tea.BlurMsg)
	return m.forwardToView(msg)
}
