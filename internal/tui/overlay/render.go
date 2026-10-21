package overlay

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// rows joins lines into an overlay view, cutting each one to width with an
// ellipsis so a narrow terminal never wraps a popup's border.
func rows(width int, lines ...string) string {
	width = max(width, 1)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		for _, part := range strings.Split(l, "\n") {
			out = append(out, ansi.Truncate(part, width, "…"))
		}
	}
	return strings.Join(out, "\n")
}
