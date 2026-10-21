package overlay

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
)

// command is the ":" bar. It is one line by design, so it sits in the
// footer rather than in a popup.
type command struct {
	input textinput.Model
}

// NewCommand opens the command bar. Enter sends CommandMsg, Esc cancels.
func NewCommand() Overlay {
	return command{input: newInput(": ", "pods, ns, dashboard", "")}
}

// Placement puts the command bar in the footer.
func (command) Placement() Placement { return Footer }

// Update edits the input until Enter or Esc.
func (c command) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case keys.IsEsc(k):
			return nil, nil
		case keys.IsEnter(k):
			return nil, send(CommandMsg{Input: c.input.Value()})
		}
	}
	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	return c, cmd
}

// View is the input line.
func (c command) View(width int) string {
	return rows(width, inputView(c.input, width))
}
