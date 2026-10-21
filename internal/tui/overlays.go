package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
)

// onOverlayInput hands a key, paste or blink to the open overlay. Input the
// overlay passes on is handled as if the overlay had not been open, so ':'
// with help open closes help and opens the command bar.
func (m Model) onOverlayInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if overlay.Passes(m.overlay, msg) {
		m.overlay = nil
		return m.Update(msg)
	}
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.Update(msg)
	return m, cmd
}

// onOverlayResult acts on what a closed overlay asked for; ok is false for
// any other message.
func (m Model) onOverlayResult(msg tea.Msg) (next tea.Model, cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case overlay.CommandMsg:
		next, cmd = m.execCmd(msg.Input)
	case overlay.TailChosenMsg:
		next, cmd = m, logsRequestCmd(msg)
	case overlay.ContainerChosenMsg:
		next, cmd = m, dispatchContainerCmd(msg.Request, msg.Container)
	case overlay.ForwardChosenMsg:
		next, cmd = m.onForwardChosen(msg)
	case overlay.RestoreDecisionMsg:
		next, cmd = m.onRestoreDecision(msg), nil
	default:
		return m, nil, false
	}
	return next, cmd, true
}
