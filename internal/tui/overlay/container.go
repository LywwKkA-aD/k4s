package overlay

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// container picks one container of a multi-container target.
type container struct {
	req    views.ContainerPromptRequestMsg
	cursor int
}

// NewContainer opens the picker for req.Containers. Arrows or j/k move,
// Enter or a digit 1-9 sends ContainerChosenMsg, Esc cancels.
func NewContainer(req views.ContainerPromptRequestMsg) Overlay {
	return container{req: req}
}

// Update moves the selection or answers.
func (c container) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	n := len(c.req.Containers)
	if n == 0 {
		return nil, nil
	}
	switch {
	case keys.IsEsc(k):
		return nil, nil
	case keys.IsEnter(k):
		return c.pick(c.cursor)
	case keys.IsArrow(k, tea.KeyUp) || k.String() == "k":
		c.cursor = (c.cursor - 1 + n) % n
		return c, nil
	case keys.IsArrow(k, tea.KeyDown) || k.String() == "j":
		c.cursor = (c.cursor + 1) % n
		return c, nil
	}
	if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		if idx := int(s[0] - '1'); idx < n {
			return c.pick(idx)
		}
	}
	return c, nil
}

func (c container) pick(idx int) (Overlay, tea.Cmd) {
	return nil, send(ContainerChosenMsg{Request: c.req, Container: c.req.Containers[idx]})
}

// View is a numbered list with the selection marked.
func (c container) View(width int) string {
	title := styles.PopupTitle.Render("container") +
		styles.Hint.Render(" · "+targetLabel(c.req.Target, c.req.Namespace, c.req.Pods))
	lines := make([]string, 0, len(c.req.Containers)+5)
	lines = append(lines, title)
	if c.req.Detached {
		lines = append(lines, styles.Warn.Render("(not the selected row)"))
	}
	lines = append(lines, "")
	for i, name := range c.req.Containers {
		marker := "  "
		label := fmt.Sprintf("%d. %s", i+1, name)
		if i == c.cursor {
			marker = styles.OK.Render("▶ ")
			label = styles.OK.Render(label)
		}
		lines = append(lines, marker+label)
	}
	lines = append(lines, "", styles.Hint.Render("↑↓/jk navigate · 1-9 quick · Enter pick · Esc cancel"))
	return rows(width, lines...)
}

// Command is what picking the highlighted container leads to: the logs at
// the tail prompt's default count, or the shell in the first pod.
func (c container) Command() kubectl.Cmd {
	r := c.req
	if len(r.Containers) == 0 {
		return kubectl.Cmd{}
	}
	name := r.Containers[c.cursor]
	switch r.NextKind {
	case "logs":
		return views.LogsCommand(r.Namespace, r.Pods, name, defaultTail, r.Logs)
	case "exec":
		if len(r.Pods) == 0 {
			return kubectl.Cmd{}
		}
		cmd, err := views.ExecCommand(views.ExecRequestMsg{Namespace: r.Namespace, Pod: r.Pods[0], Container: name})
		if err != nil {
			return kubectl.Cmd{}
		}
		return cmd
	}
	return kubectl.Cmd{}
}
