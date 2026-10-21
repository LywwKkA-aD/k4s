package events

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/table"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Column widths. LAST SEEN holds kubectl's "51s (x294 over 15h)", TYPE
// "Warning"; NAMESPACE, OBJECT and MESSAGE share what the screen has
// left, MESSAGE taking the most.
const (
	lastSeenWidth = 20
	typeWidth     = 7
	reasonWidth   = 20

	minNamespaceWidth = 9
	maxNamespaceWidth = 16
	minObjectWidth    = 16
	maxObjectWidth    = 36
	minMessageWidth   = 20

	// defaultWidth lays the table out before the first WindowSizeMsg.
	defaultWidth = 120
)

// layout fits the columns into width; narrower screens keep the minimums
// and wrap.
func layout(width int, showNamespace bool) []table.Column {
	if width <= 0 {
		width = defaultWidth
	}
	n := 5
	if showNamespace {
		n = 6
	}
	pad := styles.Table().Cell.GetHorizontalFrameSize()
	rest := width - n*pad - (lastSeenWidth + typeWidth + reasonWidth)
	cols := make([]table.Column, 0, n)
	if showNamespace {
		ns := min(max(rest/6, minNamespaceWidth), maxNamespaceWidth)
		rest -= ns
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: ns})
	}
	object := min(max(rest/3, minObjectWidth), maxObjectWidth)
	return append(cols,
		table.Column{Title: "LAST SEEN", Width: lastSeenWidth},
		table.Column{Title: "TYPE", Width: typeWidth},
		table.Column{Title: "REASON", Width: reasonWidth},
		table.Column{Title: "OBJECT", Width: object},
		table.Column{Title: "MESSAGE", Width: max(rest-object, minMessageWidth)},
	)
}

// toItems makes every cell of evs inert once: what an event says is
// whatever its writer chose.
func toItems(evs []k8s.Event, showNamespace bool) []item {
	out := make([]item, 0, len(evs))
	for _, e := range evs {
		cells := make(table.Row, 0, 6)
		if showNamespace {
			cells = append(cells, cell(e.Namespace))
		}
		cells = append(cells,
			cell(e.LastSeen),
			cell(e.Type),
			cell(e.Reason),
			cell(e.Object.Kind+"/"+e.Object.Name),
			cell(e.Message),
		)
		out = append(out, item{event: e, cells: cells})
	}
	return out
}

// cell is s on one inert line: a multi-line message keeps its words apart.
func cell(s string) string {
	return sanitize.Line(strings.ReplaceAll(s, "\n", " "))
}

// applyFilter rebuilds the rows from items through W and the filter, the
// newest maxRows of them. A non-empty keep is the row the cursor stays on
// when it is still listed.
func (m *Model) applyFilter(keep string) {
	m.typed, m.matched = 0, 0
	shown := make([]k8s.Event, 0, min(len(m.items), maxRows))
	rows := make([]table.Row, 0, cap(shown))
	for _, it := range m.items {
		if m.warnings && it.event.Type != typeWarning {
			continue
		}
		m.typed++
		if m.filter.Active() && !m.filter.Match(it.cells...) {
			continue
		}
		m.matched++
		if len(shown) < maxRows {
			shown = append(shown, it.event)
			rows = append(rows, it.cells)
		}
	}
	m.shown = shown
	m.table.SetRows(rows)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	if i := m.rowIndex(keep); i >= 0 {
		m.moveCursor(i)
		return
	}
	// An empty table leaves the cursor at -1, and SetRows does not raise
	// it again when rows come back.
	if m.table.Cursor() < 0 && len(rows) > 0 {
		m.moveCursor(0)
	}
}

// rowIndex is the index of the row keyed key, -1 when it is not listed.
func (m Model) rowIndex(key string) int {
	if key == "" {
		return -1
	}
	for i, e := range m.shown {
		if views.RowKey(e.Namespace, e.Name) == key {
			return i
		}
	}
	return -1
}

// moveCursor puts the cursor on row i by moving it, not by SetCursor: the
// table scrolls only on a move, and a far row would stay off screen.
func (m *Model) moveCursor(i int) {
	cur := m.table.Cursor()
	switch {
	case cur < 0:
		m.table.SetCursor(0)
		m.table.MoveDown(i)
	case i > cur:
		m.table.MoveDown(i - cur)
	case i < cur:
		m.table.MoveUp(cur - i)
	}
}

// current is the event under the cursor.
func (m Model) current() (k8s.Event, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.shown) {
		return k8s.Event{}, false
	}
	return m.shown[i], true
}

// selectedKey is views.RowKey of the event under the cursor, "" for none.
func (m Model) selectedKey() string {
	e, ok := m.current()
	if !ok {
		return ""
	}
	return views.RowKey(e.Namespace, e.Name)
}

// note is the line above the table that says what the rows leave out:
// the namespace a forbidden -A fell back to, and the events past maxRows
// or past what was read.
func (m Model) note() string {
	parts := make([]string, 0, 3)
	if m.fellBack {
		parts = append(parts, "namespace "+m.namespace+": cluster-wide list forbidden")
	}
	if m.loaded && m.matched > len(m.shown) {
		parts = append(parts, fmt.Sprintf("showing newest %d of %d events", len(m.shown), m.matched))
	}
	if m.loaded && m.cut {
		parts = append(parts, fmt.Sprintf("list cut at %d events", k8s.MaxEvents))
	}
	return strings.Join(parts, " · ")
}
