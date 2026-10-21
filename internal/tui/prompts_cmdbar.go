package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
)

// openCommandBar opens the ":" bar in the footer. A status message from an
// earlier action goes: the user has moved on.
func (m Model) openCommandBar() Model {
	m.status = m.status.Clear()
	m.overlay = overlay.NewCommand()
	return m
}

// execCmd switches to the view the command bar input names.
func (m Model) execCmd(input string) (Model, tea.Cmd) {
	name, ok := command.Resolve(input)
	if !ok {
		return m.setError(fmt.Sprintf("unknown command: %s", strings.TrimSpace(input)))
	}
	m.status = m.status.Clear()
	next := m.switchTo(name)
	return next, next.relayoutCmd()
}
