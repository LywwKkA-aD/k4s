// Package viz draws the dashboard widgets: meters, block and braille
// sparklines, the pod phase bar, titled boxes, and the spring that animates
// them, plus the WCAG contrast formula the styles tests use.
//
// Every widget is a pure function of a width and values that returns lines
// exactly that wide. Nothing here keeps state, starts a goroutine or reads a
// clock. Colour comes in through a Theme and never carries state on its own:
// stripped of escape codes, colour output equals mono output byte for byte.
package viz

import (
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Theme holds the styles the widgets paint with. The dashboard builds one
// from the styles package for the active theme; Mono is the theme without
// colour.
type Theme struct {
	// Muted paints borders, tracks, legend labels, ▓ Completed and · other.
	Muted lipgloss.Style
	// Accent paints panel titles and the focused border.
	Accent lipgloss.Style
	// Accent2 paints the ≈ history: block and braille sparklines.
	Accent2 lipgloss.Style
	// OK and Warn paint the meter levels (a hot meter is warn, never err); OK,
	// Warn and Err paint the Running, Pending and
	// failing phases.
	OK, Warn, Err lipgloss.Style
}

// Mono returns the theme without colour. State stays readable through the
// glyphs and through the mono attributes of the styles package: warn is
// bold, err is bold and underlined.
func Mono() Theme {
	plain := lipgloss.NewStyle()
	return Theme{
		Muted:   plain,
		Accent:  plain,
		Accent2: plain,
		OK:      plain,
		Warn:    plain.Bold(true),
		Err:     plain.Bold(true).Underline(true),
	}
}

const ellipsis = "…"

// paint renders s in st, and leaves an empty s empty so a blank run gets no
// stray escape codes.
func paint(st lipgloss.Style, s string) string {
	if s == "" {
		return ""
	}
	return st.Render(s)
}

// fit cuts s to width columns, ending in an ellipsis when it had to cut,
// and pads it with spaces to exactly width.
func fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, ellipsis)
	}
	return pad(s, width)
}

// pad fills s with spaces up to width columns.
func pad(s string, width int) string {
	if gap := width - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// cut shortens s to at most width columns with an ellipsis. Text that does
// not fit in 3 columns is dropped: a lone "…" or one letter reads as noise.
func cut(s string, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	if width < 3 {
		return ""
	}
	return ansi.Truncate(s, width, ellipsis)
}

// within limits v to [0, hi] and reads NaN as 0.
func within(v, hi float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Min(math.Max(v, 0), hi)
}

// blank is height lines of width spaces.
func blank(width, height int) string {
	line := strings.Repeat(" ", width)
	lines := make([]string, height)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}
