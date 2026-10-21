package logs

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// toggleWrap flips wrap mode and resets the horizontal pan because the two
// are mutually exclusive: in wrap mode there is no "off to the right" to
// scroll to. The next syncViewport sees a clean xOffset=0.
func (m *Model) toggleWrap() {
	m.wrap = !m.wrap
	if m.wrap {
		m.xOffset = 0
	}
}

// hscroll moves the visible window by direction × hScrollStep columns.
// Clamped at 0 on the left; the right is loosely bounded by the widest
// rendered line so the user can scroll the entire content into view
// without overshooting too far into empty space.
func (m *Model) hscroll(direction int) {
	if m.wrap {
		return
	}
	m.xOffset = min(max(m.xOffset+direction*hScrollStep, 0), m.maxXOffset())
}

// maxXOffset is the rightmost xOffset that still keeps something on screen.
// We use the widest rendered line minus a fudge so the user always sees a
// few columns of context. Viewport width is treated as a soft floor.
func (m Model) maxXOffset() int {
	if m.wrap {
		return 0
	}
	widest := 0
	for _, line := range m.rendered {
		widest = max(widest, ansi.StringWidth(line))
	}
	visible := max(m.viewport.Width(), 1)
	return max(widest-visible/2, 0)
}

// syncViewport lays the backlog out as viewport rows: each line from the
// rendered cache, or painted afresh when it holds search matches, then
// wrapped or panned. It records where every line starts in rowStart, so
// n/N and a trim can work in rows.
func (m *Model) syncViewport() {
	width := m.viewport.Width()
	rows := make([]string, 0, len(m.rendered))
	starts := make([]int, len(m.rendered)+1)
	next := 0
	for i, line := range m.rendered {
		for next < len(m.matches) && m.matches[next].line < i {
			next++
		}
		from := next
		for next < len(m.matches) && m.matches[next].line == i {
			next++
		}
		if next > from {
			line = m.paintMatches(i, from, next)
		}
		starts[i] = len(rows)
		rows = m.appendRows(rows, line, width)
	}
	starts[len(m.rendered)] = len(rows)
	m.rowStart = starts
	m.viewport.SetContentLines(rows)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// paintMatches draws line i with matches[from:to], all on that line.
func (m Model) paintMatches(i, from, to int) string {
	spans := make([]span, 0, to-from)
	for _, mt := range m.matches[from:to] {
		spans = append(spans, mt.span)
	}
	current := noCurrent
	if m.matchIdx >= from && m.matchIdx < to {
		current = m.matchIdx - from
	}
	ln := m.rawLines[i]
	return formatLine(ln, len(m.pods) > 1, m.showPodNames, m.podColors[ln.pod], spans, current)
}

// appendRows adds the rows line takes on screen: several when wrapped,
// one panned by xOffset otherwise.
func (m Model) appendRows(rows []string, line string, width int) []string {
	switch {
	case m.wrap && width > 0:
		return append(rows, strings.Split(wrapLine(line, width), "\n")...)
	case m.xOffset > 0:
		return append(rows, applyXOffset(line, m.xOffset))
	}
	return append(rows, line)
}

// applyXOffset drops the first n display columns of an ANSI-aware string.
// ansi.TruncateLeft preserves SGR continuity so colour codes survive the
// crop. n=0 is the identity.
func applyXOffset(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.TruncateLeft(s, n, "")
}

// wrapLine soft-wraps an ANSI-coloured string to width. Reuses the same
// charmbracelet/x/ansi helper as the describe view — consistent behaviour
// across the app. rowWithin wraps the same way.
func wrapLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Wrap(s, width, "")
}

// View implements views.View.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}

	body := m.viewport.View()
	if len(m.rawLines) == 0 {
		body = styles.Hint.Render(fmt.Sprintf("waiting for logs… (tail %d)", m.tail))
	}

	top := make([]string, 0, statusRowsOpen)
	if m.searchOpen {
		top = append(top, m.searchInput.View(), "")
	} else {
		top = append(top, m.statusLine())
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(top, body)...)
}

// statusLine renders the one-line status above the viewport: any combination
// of "[paused]", "[tags on]", "[wrap on]", "[→ N]" and "/query · M/N".
// Empty when nothing applies — the row stays present so the layout does
// not jiggle.
func (m Model) statusLine() string {
	bits := make([]string, 0, 4)
	if m.previous {
		bits = append(bits, styles.Hint.Render("[previous instance — press p for the current one]"))
	}
	if !m.autoFollow {
		bits = append(bits, styles.Warn.Render("[paused — press f to follow]"))
	}
	if m.wrap {
		bits = append(bits, styles.Hint.Render("[wrap on — press w to disable]"))
	} else if m.xOffset > 0 {
		bits = append(bits, styles.Hint.Render(fmt.Sprintf("[→ %d cols]", m.xOffset)))
	}
	if len(m.pods) > 1 && m.showPodNames {
		bits = append(bits, styles.Hint.Render("[tags on — press t to compact]"))
	}
	switch {
	case m.search.text != "" && len(m.matches) > 0:
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · %d/%d (n/N step · c cancel)", m.search.text, m.matchIdx+1, len(m.matches))))
	case m.search.text != "":
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · no match (c cancel)", m.search.text)))
	}
	return strings.Join(bits, " · ")
}
