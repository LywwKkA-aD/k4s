package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// copyCommand puts the footer's command on the clipboard as it would be
// pasted: decorated for this cluster and without the approximate mark,
// which the status row spells out instead.
// The copy goes through the terminal (OSC 52), so it works over SSH and,
// with `set -g set-clipboard on`, inside tmux. The status row repeats it
// with $HOME shortened to ~.
func (m Model) copyCommand() (Model, tea.Cmd) {
	cmd := m.decorate(m.activeCommand())
	text := cmd.String()
	if text == "" {
		return m.setStatus(status.Info, "nothing to copy")
	}
	verb := "copied"
	if cmd.Approx {
		verb = "copied (approximate)"
	}
	next, expire := m.setStatus(status.Info, verb+": "+text)
	return next, tea.Batch(tea.SetClipboard(text), expire)
}
