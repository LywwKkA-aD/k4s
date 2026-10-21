package dashboard

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// cols is the measured layout of a table: each column as wide as its
// widest cell or header, right-aligned or not, and two spaces after it,
// one after a tight column (a meter ends in its own blank mark column).
type cols struct {
	widths []int
	right  []bool
	tight  []bool
}

// measure lays out a table of head and rows; right and tight pick columns
// by their header, and nil picks none.
func measure(head []string, rows [][]string, right, tight func(string) bool) cols {
	c := cols{widths: make([]int, len(head)), right: make([]bool, len(head)), tight: make([]bool, len(head))}
	for i, h := range head {
		c.widths[i] = ansi.StringWidth(h)
		c.right[i] = right != nil && right(h)
		c.tight[i] = tight != nil && tight(h)
	}
	for _, r := range rows {
		for i, cell := range r {
			c.widths[i] = max(c.widths[i], ansi.StringWidth(cell))
		}
	}
	return c
}

func (c cols) gap(i int) string {
	if c.tight[i] {
		return " "
	}
	return "  "
}

// width is the width of a full row.
func (c cols) width() int {
	total := 0
	for i, w := range c.widths {
		total += w
		if i < len(c.widths)-1 {
			total += len(c.gap(i))
		}
	}
	return total
}

// join lays out one row: each cell padded to its column, right-aligned
// where the column is, the last left-aligned cell left unpadded.
func (c cols) join(cells []string) string {
	var b strings.Builder
	for i, cell := range cells {
		switch {
		case c.right[i]:
			b.WriteString(padLeft(cell, c.widths[i]))
		case i == len(cells)-1:
			b.WriteString(cell)
		default:
			b.WriteString(padRight(cell, c.widths[i]))
		}
		if i < len(cells)-1 {
			b.WriteString(c.gap(i))
		}
	}
	return b.String()
}

func padLeft(s string, w int) string {
	if gap := w - ansi.StringWidth(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}
	return s
}

// window is the first of total rows to draw in room lines so the row at
// sel (-1 for none) is on screen.
func window(total, room, sel int) int {
	if room <= 0 || total <= room || sel < room {
		return 0
	}
	return min(sel-room+1, total-room)
}

// listLine is one body line: the cursor column, then text. The selected
// line is drawn plain in the selection style behind the cursor marker, so
// the cursor shows in every theme; mono draws it in reverse video.
func (f frame) listLine(text string, sel bool) string {
	if sel {
		return f.ds.Selection.Render(styles.CursorMarker + ansi.Strip(text))
	}
	return " " + text
}

// atLeast returns c with column i at least w wide, so a column keeps its
// place while the rows that set its width come and go.
func (c cols) atLeast(i, w int) cols {
	widths := slices.Clone(c.widths)
	widths[i] = max(widths[i], w)
	c.widths = widths
	return c
}

// fitHead is the first of steps (the lines above a list, fullest first)
// that leaves need of h lines for the list, or nothing: a list loses rows
// down to need before the lines above it go.
func fitHead(h, need int, steps ...[]string) []string {
	for _, s := range steps {
		if h-len(s) >= need {
			return s
		}
	}
	return nil
}

// selectedIn is the index of the row the cursor stands on in panel p, -1
// when the cursor is elsewhere.
func (f frame) selectedIn(p dash.Panel, rows []row) int {
	if f.focus.panel != p || f.focus.row == "" {
		return -1
	}
	return slices.IndexFunc(rows, func(r row) bool { return r.key == f.focus.row })
}

// listFooter is a list's bottom border note: the cursor's place, the rows
// left out, then notes.
func (f frame) listFooter(p dash.Panel, total, hidden int, notes ...string) string {
	var parts []string
	if f.focus.panel == p && f.focus.row != "" {
		parts = append(parts, fmt.Sprintf("%d/%d", f.focus.index+1, total))
	}
	if hidden > 0 {
		parts = append(parts, fmt.Sprintf("+%d", hidden))
	}
	for _, n := range notes {
		if n != "" {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, " · ")
}
