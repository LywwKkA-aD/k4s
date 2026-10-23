package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// setStatus shows text in the footer's status row and arms its expiry.
func (m Model) setStatus(level status.Level, text string) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.status, cmd = m.status.Set(level, text, m.now())
	return m, cmd
}

// setStatusAbout is setStatus for a message about c: while it shows, the
// footer's command row shows c and y copies it (see activeCommand).
func (m Model) setStatusAbout(level status.Level, text string, c kubectl.Cmd) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.status, cmd = m.status.SetCommand(level, text, c, m.now())
	return m, cmd
}

// setToast is setStatus for news nobody asked for (views.ToastMsg): it
// never replaces an error still showing, which waits for the user's key.
func (m Model) setToast(level status.Level, text string) (Model, tea.Cmd) {
	if cur, ok := m.status.Current(m.now()); ok && cur.Level == status.Error {
		return m, nil
	}
	return m.setStatus(level, text)
}

// setError is setStatus at status.Error.
func (m Model) setError(text string) (Model, tea.Cmd) {
	return m.setStatus(status.Error, text)
}

// statusStyle colours a status row by level.
func statusStyle(level status.Level) lipgloss.Style {
	switch level {
	case status.OK:
		return styles.OK
	case status.Warn:
		return styles.Warn
	case status.Error, status.Alert:
		return styles.Err
	}
	return lipgloss.NewStyle()
}
