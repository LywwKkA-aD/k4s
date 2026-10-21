package pods

import (
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// View renders the table or a placeholder, under the filter bar when a
// filter is open or committed. A restored filter shows from the first
// frame, before its rows arrive.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.filter.Active() {
		return m.body()
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.filter.View(), m.body())
}

func (m Model) body() string {
	switch {
	case !m.loaded:
		return styles.Hint.Render("loading pods…")
	case m.err != nil:
		return styles.Warn.Render("pods unavailable: " + m.err.Error())
	case len(m.table.Rows()) > 0:
		return m.table.View()
	case m.filter.Active():
		return styles.Hint.Render("no pods match /" + m.filter.Query())
	}
	return styles.Hint.Render("no pods")
}
