package events

import (
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// View renders the table or a placeholder, under the filter bar, the
// stale banner and the note when there are any. A restored filter shows
// from the first frame, before its rows arrive.
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
	if note := m.note(); note != "" {
		above = append(above, styles.Hint.Render(note))
	}
	if len(above) == 0 {
		return m.body()
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(above, m.body())...)
}

func (m Model) body() string {
	switch {
	case !m.loaded:
		return styles.Hint.Render("loading events…")
	case m.health.Failed():
		text := styles.Warn.Render("events unavailable: " + m.health.Text())
		if d := detail(m.health.Err()); d != "" {
			text = lipgloss.JoinVertical(lipgloss.Left, text, styles.Hint.Render(d))
		}
		return text
	case len(m.shown) > 0:
		return m.table.View()
	case m.filter.Active():
		return styles.Hint.Render("no events match /" + m.filter.Query())
	}
	return styles.Hint.Render("no events")
}
