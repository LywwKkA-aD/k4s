package overlay

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
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
// TailChosenMsg with the count clamped to the per-pod cap the prompt
// names; Esc cancels.
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
			return nil, send(TailChosenMsg{Request: t.req, Tail: t.count()})
		}
	}
	var cmd tea.Cmd
	t.input, cmd = t.input.Update(msg)
	return t, cmd
}

// View names the target in the title, then the input and its keys. A
// request made for a row the cursor has since left says so under the
// title, so the user can still back out with Esc.
func (t tail) View(width int) string {
	title := styles.PopupTitle.Render("tail lines") +
		styles.Hint.Render(" · "+targetLabel(t.req.Target, t.req.Namespace, t.req.Pods))
	lines := []string{title}
	if t.req.Detached {
		lines = append(lines, styles.Warn.Render("(not the selected row)"))
	}
	lines = append(lines,
		"",
		inputView(t.input, width),
		"",
		styles.Hint.Render("Enter run · Esc cancel · default 100"),
		styles.Hint.Render(capHint(views.StreamedPods(len(t.req.Pods)))),
	)
	return rows(width, lines...)
}

// Command is the logs command Enter would open at the count typed so far.
func (t tail) Command() kubectl.Cmd {
	r := t.req
	return views.LogsCommand(r.Namespace, r.Pods, r.Container, t.count(), r.Logs)
}

// count is the line count typed so far, clamped to what the logs view
// keeps (views.ClampTail) over the pods it streams, so the footer previews
// what will run.
func (t tail) count() int64 {
	return views.ClampTail(parseTail(t.input.Value()), views.StreamedPods(len(t.req.Pods)))
}

// capHint names the tail cap: the logs view keeps views.MaxLogLines, shared
// out across the pods it streams.
func capHint(pods int) string {
	limit := strconv.FormatInt(views.TailCap(pods), 10)
	if pods <= 1 {
		return "max " + limit + " lines"
	}
	return "max " + limit + " per pod, " + strconv.Itoa(views.MaxLogLines) + " in all"
}

// TakesText reports the count input: y is text there.
func (tail) TakesText() bool { return true }

// parseTail reads a positive line count; a casual prompt never surfaces a
// hard error, so anything else is the default.
func parseTail(value string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		return defaultTail
	}
	return n
}
