package tuitest

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Strip removes ANSI escape sequences, so assertions and goldens compare
// text only and survive colour or renderer changes.
func Strip(view string) string {
	return ansi.Strip(view)
}

// Frame returns what v renders, escape codes included: the body of a view
// (View() string) or the content of the root model's frame (View()
// tea.View). Anything else panics: it is a mistake in the test.
func Frame(v any) string {
	switch v := v.(type) {
	case interface{ View() string }:
		return v.View()
	case interface{ View() tea.View }:
		return v.View().Content
	}
	panic(fmt.Sprintf("tuitest: %T does not render a frame", v))
}

// Screen renders v and strips it.
func Screen(v any) string {
	return Strip(Frame(v))
}

// Footer renders v, the root model, and returns what lies under the
// footer's top rule: the hints or status row and the kubectl row, trailing
// blanks trimmed. It is empty when the screen has no rule.
func Footer(v any) string {
	lines := strings.Split(Screen(v), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if s := strings.TrimSpace(lines[i]); s != "" && strings.Trim(s, "─") == "" {
			return TrimLines(strings.Join(lines[i+1:], "\n"))
		}
	}
	return ""
}

// TrimLines strips trailing spaces from every line and drops trailing empty
// lines. Width-padded renders then compare by content alone.
func TrimLines(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, strings.TrimRight(l, " "))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}
