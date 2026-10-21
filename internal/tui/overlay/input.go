package overlay

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	inputWidth = 40
	inputLimit = 64
)

// newInput returns a focused text input. The cursor starts steady: the blink
// Cmd Focus returns is dropped, and the first edit starts blinking.
func newInput(prompt, placeholder, value string) textinput.Model {
	ti := textinput.New()
	ti.CharLimit = inputLimit
	ti.SetWidth(inputWidth)
	ti.SetStyles(styles.TextInput())
	ti.Prompt = prompt
	ti.Placeholder = placeholder
	ti.SetValue(value)
	ti.CursorEnd()
	ti.Focus()
	return ti
}

// inputView renders ti in at most width columns. bubbles pads a placeholder
// to the input width plus the prompt; k4s fits it, prompt included, into the
// input width, so a popup narrows to its other lines once the input is
// emptied.
func inputView(ti textinput.Model, width int) string {
	promptWidth := lipgloss.Width(ti.Prompt)
	w := ti.Width()
	if ti.Value() == "" && ti.Placeholder != "" {
		w -= promptWidth + 1
	}
	w = min(w, width-promptWidth-1)
	ti.SetWidth(max(w, 1))
	return ti.View()
}

// targetLabel names what a prompt acts on: the row it was asked for when
// the request says, else its pods, then the namespace.
func targetLabel(target views.ResourceRef, namespace string, pods []string) string {
	name := podsLabel(pods)
	if target.Name != "" {
		name = target.Name
		if target.Kind != "" {
			name = target.Kind + "/" + target.Name
		}
	}
	if target.Namespace != "" {
		namespace = target.Namespace
	}
	if namespace == "" {
		return name
	}
	return name + " · ns " + namespace
}

func podsLabel(pods []string) string {
	if len(pods) == 1 {
		return "pod/" + pods[0]
	}
	return fmt.Sprintf("%d pods", len(pods))
}
