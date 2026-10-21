package overlay

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// defaultTail is the prefilled line count, and what anything that is not a
// positive number falls back to.
const defaultTail = 100

// tail asks how many lines each pod's log starts with.
type tail struct {
	req   views.TailPromptRequestMsg
	input textinput.Model
}

// NewTail opens the tail prompt for req, prefilled with 100. Enter sends
// TailChosenMsg, Esc cancels.
func NewTail(req views.TailPromptRequestMsg) Overlay {
	n := strconv.Itoa(defaultTail)
	return tail{req: req, input: newInput("❯ ", n, n)}
}

// Update edits the count until Enter or Esc.
func (t tail) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case keys.IsEsc(k):
			return nil, nil
		case keys.IsEnter(k):
			return nil, send(TailChosenMsg{Request: t.req, Tail: parseTail(t.input.Value())})
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, cmd
}

// View names the target in the title, then the input and its keys.
func (t tail) View(width int) string {
	title := styles.PopupTitle.Render("tail lines") +
		styles.Hint.Render(" · "+targetLabel(t.req.Target, t.req.Namespace, t.req.Pods))
	return rows(width,
		title,
		"",
		inputView(t.input, width),
		"",
		styles.Hint.Render("Enter run · Esc cancel · default 100"),
	)
}

// parseTail reads a positive line count; a casual prompt never surfaces a
// hard error, so anything else is the default.
func parseTail(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		return defaultTail
	}
	return n
}
