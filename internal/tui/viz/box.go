package viz

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Box is a dashboard panel frame with its title in the top edge:
//
//	╭─┤ 1 cluster ├───── ◆ k3s ─╮
//	│ ● api  ready              │
//	╰──────── ctx growlytics ───╯
//
// Title, Info and Footer are plain text; the box paints them.
type Box struct {
	// Title sits at the left of the top edge, such as "3 pods".
	Title string
	// Info sits at the right of the top edge, such as "185 · 33 ns".
	Info string
	// Footer sits at the right of the bottom edge, such as "+3 · z zoom".
	Footer string
	// Focused draws a heavy border and a bold title, in every theme, so
	// focus shows without colour.
	Focused bool
}

// edges are the glyphs of one border weight.
type edges struct {
	h, v, tl, tr, bl, br string
	// open and close bracket the title: ┤ title ├.
	open, close string
}

func (b Box) edges() edges {
	border := lipgloss.RoundedBorder()
	if b.Focused {
		border = lipgloss.ThickBorder()
	}
	return edges{
		h: border.Top, v: border.Left,
		tl: border.TopLeft, tr: border.TopRight,
		bl: border.BottomLeft, br: border.BottomRight,
		open: border.MiddleRight, close: border.MiddleLeft,
	}
}

// Render frames body in a box exactly width columns by height rows. Text
// that does not fit in an edge is cut with an ellipsis, Info before the
// title, and dropped below 3 columns. Body lines may carry styles; they are
// cut to width-2 with an ellipsis and padded, lines past height-2 are
// dropped and missing ones are blank. The box adds no inner padding: the
// first column of a line is the caller's, for a margin or the row cursor.
// A box one row tall is its top edge; one column wide, it is blank.
func (b Box) Render(th Theme, width, height int, body string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if width < 2 {
		return blank(width, height)
	}
	e := b.edges()
	border, title := th.Muted, th.Accent
	if b.Focused {
		border, title = th.Accent, th.Accent.Bold(true)
	}
	inner := width - 2
	lines := make([]string, 0, height)
	lines = append(lines, b.top(e, border, title, th.Muted, inner))
	if height == 1 {
		return lines[0]
	}
	rows := strings.Split(body, "\n")
	side := paint(border, e.v)
	for i := range height - 2 {
		row := ""
		if i < len(rows) {
			row = rows[i]
		}
		lines = append(lines, side+fit(row, inner)+side)
	}
	note, used := edgeNote(e, border, th.Muted, b.Footer, inner-1)
	lines = append(lines, paint(border, e.bl+strings.Repeat(e.h, inner-used))+note+paint(border, e.br))
	return strings.Join(lines, "\n")
}

func (b Box) top(e edges, border, title, muted lipgloss.Style, inner int) string {
	head, used := "", 0
	// The title takes "─┤ " and " ├" around it.
	if t := cut(b.Title, inner-5); t != "" {
		head = paint(border, e.h+e.open) + " " + paint(title, t) + " " + paint(border, e.close)
		used = ansi.StringWidth(t) + 5
	}
	// One fill cell keeps the info apart from the title.
	note, noteW := edgeNote(e, border, muted, b.Info, inner-used-1)
	fill := paint(border, strings.Repeat(e.h, inner-used-noteW))
	return paint(border, e.tl) + head + fill + note + paint(border, e.tr)
}

// edgeNote is " text ─" for the right end of an edge with room columns,
// and its width; empty when the text does not fit.
func edgeNote(e edges, border, muted lipgloss.Style, text string, room int) (string, int) {
	t := cut(text, room-3)
	if t == "" {
		return "", 0
	}
	return " " + paint(muted, t) + " " + paint(border, e.h), ansi.StringWidth(t) + 3
}
