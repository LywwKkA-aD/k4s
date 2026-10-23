package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onOpenView opens the list a view asked for (the dashboard's panels and
// rows) through the command bar's paths: a namespaced list with its
// namespace as `:<name> <ns>` (openIn), a cluster-wide one as `:<name>`,
// then in the state asked for.
func (m Model) onOpenView(msg views.OpenViewMsg) (tea.Model, tea.Cmd) {
	name, ok := command.Resolve(msg.Name)
	if !ok || !isRebuildableView(name) || name == viewDashboard {
		return m.setStatus(status.Warn, "no k4s view for "+sanitize.Line(msg.Name))
	}
	var open tea.Cmd
	if isNamespacedList(name) {
		m, open = m.openIn(name, msg.Namespace)
	} else {
		m = m.switchTo(name)
		open = m.relayoutCmd()
	}
	m, restored := m.restore(msg.State)
	return m, tea.Batch(restored, open)
}
