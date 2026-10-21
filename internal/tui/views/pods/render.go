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
	above := make([]string, 0, 3)
	if m.filter.Active() {
		above = append(above, m.filter.View())
	}
	if banner := m.health.Banner(); banner != "" {
		above = append(above, banner)
	}
	if len(above) == 0 {
		return m.body()
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(above, m.body())...)
}

func (m Model) body() string {
	switch {
	case !m.loaded:
		return styles.Hint.Render("loading pods…")
	case m.health.Failed():
		return styles.Warn.Render("pods unavailable: " + m.health.Text())
	case len(m.table.Rows()) > 0:
		return m.table.View()
	case m.filter.Active():
		return styles.Hint.Render("no pods match /" + m.filter.Query())
	}
	return styles.Hint.Render("no pods")
}
