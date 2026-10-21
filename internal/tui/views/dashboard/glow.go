package dashboard

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Gradient sweeps paint with styles.Gradient, the accent to accent2 blend
// whose every stop the contrast tests cover. A sweep only recolours:
// stripped of its escapes, a frame mid-sweep reads exactly like one at
// rest, so mono and NO_COLOR lose nothing.

// edgeRunes are the box-drawing glyphs of a panel's edges, light and heavy.
const edgeRunes = "─━│┃╭╮╰╯┏┓┗┛┤├┫┣"

// glowText paints each cell of s from the gradient spread over width
// cells and shifted by offset.
func glowText(s string, width, offset int) string {
	ramp := styles.Gradient(max(width, 1))
	var b strings.Builder
	i := 0
	for _, r := range s {
		if r == ' ' {
			b.WriteRune(r)
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(ramp.At(i + offset)).Render(string(r)))
		}
		i++
	}
	return b.String()
}

// sweepEdges repaints the top and bottom edges of the rendered box: the
// edge glyphs from the gradient shifted by offset, the title in the
// focused title style and the notes in the label style.
func sweepEdges(box, title string, ds styles.DashboardStyles, offset int) string {
	lines := strings.Split(box, "\n")
	for _, i := range []int{0, len(lines) - 1} {
		lines[i] = sweepEdge(ansi.Strip(lines[i]), title, ds, offset)
	}
	return strings.Join(lines, "\n")
}

// sweepEdge repaints one plain edge line.
func sweepEdge(line, title string, ds styles.DashboardStyles, offset int) string {
	ramp := styles.Gradient(utf8.RuneCountInString(line))
	from, to := -1, -1
	if at := strings.Index(line, " "+title+" "); at >= 0 && title != "" {
		from = utf8.RuneCountInString(line[:at]) + 1
		to = from + utf8.RuneCountInString(title)
	}
	var b strings.Builder
	var run []rune
	runTitle := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		st := ds.Label
		if runTitle {
			st = ds.TitleFocused
		}
		b.WriteString(st.Render(string(run)))
		run = run[:0]
	}
	i := 0
	for _, r := range line {
		switch {
		case strings.ContainsRune(edgeRunes, r):
			flush()
			b.WriteString(lipgloss.NewStyle().Foreground(ramp.At(i + offset)).Render(string(r)))
		case r == ' ':
			flush()
			b.WriteRune(r)
		default:
			if inTitle := i >= from && i < to; inTitle != runTitle {
				flush()
				runTitle = inTitle
			}
			run = append(run, r)
		}
		i++
	}
	flush()
	return b.String()
}
