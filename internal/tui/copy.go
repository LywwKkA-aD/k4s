package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// copyCommand puts the footer's command on the clipboard as it would be
// pasted: decorated for this cluster and without the approximate mark,
// which the status row spells out instead.
// The copy goes through the terminal (OSC 52), so it works over SSH and,
// with `set -g set-clipboard on`, inside tmux. The status row repeats it
// with $HOME shortened to ~.
//
// It copies what the command row showed as y was pressed: y can be the
// key that dismisses an error, and a command that error brought to the row
// (the kubectl debug command) stays there while the copy's status shows.
func (m Model) copyCommand() (Model, tea.Cmd) {
	shown, about := m.commandOver(m.status.CommandAtKey(m.now()))
	cmd := m.decorate(shown)
	text := cmd.String()
	if text == "" {
		return m.setStatus(status.Info, "nothing to copy")
	}
	verb := "copied"
	if cmd.Approx {
		verb = "copied (approximate)"
	}
	var keep kubectl.Cmd
	if about {
		keep = shown
	}
	next, expire := m.setStatusAbout(status.Info, verb+": "+text, keep)
	return next, tea.Batch(tea.SetClipboard(text), expire)
}
