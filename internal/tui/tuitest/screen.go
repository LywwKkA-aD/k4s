package tuitest

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Viewer is anything that renders a frame: the root model and every view.
type Viewer interface {
	View() string
}

// Strip removes ANSI escape sequences, so assertions and goldens compare
// text only and survive colour or renderer changes.
func Strip(view string) string {
	return ansi.Strip(view)
}

// Screen renders v and strips it.
func Screen(v Viewer) string {
	return Strip(v.View())
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
