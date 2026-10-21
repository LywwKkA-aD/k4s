package forwards

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// unavailable is shown, and said by every row key, when the port-forward
// state could not be loaded at startup.
const unavailable = "port-forward subsystem unavailable"

type keyMap struct {
	start   key.Binding
	stop    key.Binding
	restart key.Binding
	remove  key.Binding
	refresh key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		start:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "start")),
		stop:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "stop")),
		restart: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "restart")),
		remove:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "delete")),
		refresh: key.NewBinding(key.WithKeys("ctrl+r"), key.WithHelp("ctrl+r", "refresh")),
	}
}

func (k keyMap) help() []key.Binding {
	return []key.Binding{k.start, k.stop, k.restart, k.remove, k.refresh}
}

// handleKey runs the view's own keys; handled is false for any other key,
// which then moves the table cursor. A row key that cannot act says why on
// the status line.
func (m Model) handleKey(msg tea.KeyPressMsg) (next Model, cmd tea.Cmd, handled bool) {
	if key.Matches(msg, m.keys.refresh) {
		m.refresh()
		return m, nil, true
	}
	if !key.Matches(msg, m.keys.start, m.keys.stop, m.keys.restart, m.keys.remove) {
		return m, nil, false
	}
	if m.manager == nil {
		return m, statusCmd(views.StatusMsg{Level: status.Warn, Text: unavailable}), true
	}
	f, ok := m.selected()
	if !ok {
		return m, statusCmd(views.StatusMsg{Level: status.Warn, Text: "no port-forward selected"}), true
	}
	switch {
	case key.Matches(msg, m.keys.start):
		return m, startCmd(m.manager, f), true
	case key.Matches(msg, m.keys.stop):
		return m, stopCmd(m.manager, f), true
	case key.Matches(msg, m.keys.restart):
		return m, restartCmd(m.manager, f), true
	}
	return m, removeCmd(m.manager, f), true
}
