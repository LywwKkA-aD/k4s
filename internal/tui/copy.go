package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// copyCommand puts the footer's command on the clipboard as it would be
// pasted: decorated for this cluster and without the approximate mark.
// The copy goes through the terminal (OSC 52), so it works over SSH and,
// with `set -g set-clipboard on`, inside tmux. The status row repeats it
// with $HOME shortened to ~.
func (m Model) copyCommand() (Model, tea.Cmd) {
	text := m.decorate(m.activeCommand()).String()
	if text == "" {
		return m.setStatus(status.Info, "nothing to copy")
	}
	next, expire := m.setStatus(status.Info, "copied: "+text)
	return next, tea.Batch(tea.SetClipboard(text), expire)
}
