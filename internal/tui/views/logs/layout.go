package logs

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// maxBacklogBytes bounds the text the backlog keeps, next to
// views.MaxLogLines: 5000 lines of up to 64 KiB each would hold 320 MiB,
// and the viewport measures every row it is given on each batch.
const maxBacklogBytes = 8 << 20

// cell is one line laid out as viewport rows, and the search paint they
// were drawn with: noPaint, or the paint generation of the matches on it.
type cell struct {
	rows  []string
	paint uint64
}

const noPaint = 0

// layoutKey is what a line's rows depend on besides its own text and
// paint. A change of key lays every line out again.
type layoutKey struct {
	wrap    bool
	width   int
	xOffset int
}

func (m Model) layoutKey() layoutKey {
	return layoutKey{wrap: m.wrap, width: m.viewport.Width(), xOffset: m.xOffset}
}

// syncViewport lays the backlog out as viewport rows: each line from the
// cell it was laid out in, or laid out now when it is new, the layout
// changed, or the search paint on it did. A batch therefore costs its own
// lines, not the backlog. rowStart records where every line starts, so
// n/N and a trim can work in rows.
func (m *Model) syncViewport() {
	key := m.layoutKey()
	old := m.cells
	if key != m.cellsKey {
		old = nil
	}
	cells := make([]cell, len(m.rendered))
	starts := make([]int, len(m.rendered)+1)
	rows := make([]string, 0, len(m.rendered))
	var p *painter
	next := 0
	for i := range m.rendered {
		for next < len(m.matches) && m.matches[next].line < i {
			next++
		}
		from := next
		for next < len(m.matches) && m.matches[next].line == i {
			next++
		}
		paint := uint64(noPaint)
		if next > from {
			paint = m.paintGen
		}
		if i < len(old) && old[i].rows != nil && old[i].paint == paint {
			cells[i] = old[i]
		} else {
			line := m.rendered[i]
			if paint != noPaint {
				if p == nil {
					fresh := newPainter()
					p = &fresh
				}
				line = m.paintMatches(*p, i, from, next)
			}
			cells[i] = cell{rows: layoutRows(line, key), paint: paint}
		}
		starts[i] = len(rows)
		rows = append(rows, cells[i].rows...)
	}
	starts[len(m.rendered)] = len(rows)
	m.cells, m.cellsKey, m.rowStart = cells, key, starts
	m.viewport.SetContentLines(rows)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// repaint retires the search paint of every line: the matches or the
// current one changed, so painted lines are drawn again at the next sync.
func (m *Model) repaint() {
	m.paintGen++
}

// paintMatches draws line i with matches[from:to], all on that line.
func (m Model) paintMatches(p painter, i, from, to int) string {
	spans := make([]span, 0, to-from)
	for _, mt := range m.matches[from:to] {
		spans = append(spans, mt.span)
	}
	current := noCurrent
	if m.matchIdx >= from && m.matchIdx < to {
		current = m.matchIdx - from
	}
	return p.line(m.rawLines[i], m.rendered[i], spans, current)
}

// layoutRows is the rows line takes on screen: soft-wrapped to the width,
// or, clipped, the window from xOffset that the viewport shows. Cutting
// here keeps a 64 KiB line from being measured whole on every batch.
func layoutRows(line string, key layoutKey) []string {
	switch {
	case key.width <= 0:
		return []string{line}
	case key.wrap:
		return strings.Split(wrapLine(line, key.width), "\n")
	}
	return []string{ansi.Cut(line, key.xOffset, key.xOffset+key.width)}
}
