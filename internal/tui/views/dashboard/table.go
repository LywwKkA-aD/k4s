package dashboard

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

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
