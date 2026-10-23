package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onOpenView opens the list a view asked for (the dashboard's panels and
// rows) through the command bar's paths: a namespaced list with its
// namespace as `:<name> <ns>` (openIn), a cluster-wide one as `:<name>`,
// then in the state asked for. The namespace can come from a row the
// cluster named and skips command.Parse, so it is checked here.
func (m Model) onOpenView(msg views.OpenViewMsg) (tea.Model, tea.Cmd) {
	name, ok := command.Resolve(msg.Name)
	if !ok || !isRebuildableView(name) || name == viewDashboard {
		return m.setStatus(status.Warn, "no k4s view for "+sanitize.Line(msg.Name))
	}
	if isNamespacedList(name) && msg.Namespace != "" {
		if err := kubectl.CheckLabel("namespace", msg.Namespace); err != nil {
			return m.setStatus(status.Warn, "no k4s view for "+name+": "+sanitize.Line(err.Error()))
		}
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
