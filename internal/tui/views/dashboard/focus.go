package dashboard

import (
	"slices"
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
)

// focus is where the cursor stands: a panel (0 is the banner) and, inside
// a list panel, a row. The row is named by its key, never by its index:
// a poll re-sorts the rows, and the cursor must stay on the object the
// user picked (critique L5/L6).
type focus struct {
	panel dash.Panel
	// row is the key of the row under the cursor, "" at panel level.
	row string
	// index is the row's position when it was last placed: where the
	// cursor lands when the row goes away.
	index int
	// expanded is the panel the compact accordion keeps open; the latest
	// panel focused, and before that the first snapshot's pick.
	expanded dash.Panel
}

// row is one line the cursor can stand on and the command it stands for.
type row struct {
	key string
	cmd dash.Cmd
	// target is the object behind the row, for Enter; zero for rows such
	// as the cluster's api line.
	target dash.ObjectKey
	// placed is set when target carries its namespace.
	placed bool
}

// at focuses panel p at panel level. Focusing a panel opens it in the
// accordion; the banner leaves the open panel as it is.
func (f focus) at(p dash.Panel) focus {
	f.panel, f.row, f.index = p, "", 0
	if p != dash.PanelOverview {
		f.expanded = p
	}
	return f
}

// cycle moves step panels along order, wrapping around. A panel not in
// order (hidden at this size) counts as the banner.
func (f focus) cycle(order []dash.Panel, step int) focus {
	if len(order) == 0 {
		return f
	}
	i := max(slices.Index(order, f.panel), 0)
	n := len(order)
	return f.at(order[((i+step)%n+n)%n])
}

// down moves the cursor one row down, from the panel level onto the
// first row; it stays on the last row.
func (f focus) down(rows []row) focus {
	if len(rows) == 0 {
		return f
	}
	if f.row == "" {
		return f.onRow(rows, 0)
	}
	i := f.indexIn(rows)
	return f.onRow(rows, min(i+1, len(rows)-1))
}

// up moves the cursor one row up; from the first row it goes back to the
// panel level.
func (f focus) up(rows []row) focus {
	if f.row == "" {
		return f
	}
	i := f.indexIn(rows)
	if i <= 0 {
		f.row, f.index = "", 0
		return f
	}
	return f.onRow(rows, i-1)
}

// back is one step of the Esc chain: row to panel, panel to the banner.
// used is false on the banner, where Esc belongs to the root.
func (f focus) back() (focus, bool) {
	switch {
	case f.row != "":
		f.row, f.index = "", 0
		return f, true
	case f.panel != dash.PanelOverview:
		return f.at(dash.PanelOverview), true
	}
	return f, false
}

// anchor puts the cursor back on its row after rows changed. When that row
// is gone the cursor lands on the row now at its old index (the last row
// if the list got shorter, the panel when it is empty) and gone names the
// row that left.
func (f focus) anchor(rows []row) (next focus, gone string) {
	if f.row == "" {
		return f, ""
	}
	if i := slices.IndexFunc(rows, func(r row) bool { return r.key == f.row }); i >= 0 {
		f.index = i
		return f, ""
	}
	gone = f.row
	if len(rows) == 0 {
		f.row, f.index = "", 0
		return f, gone
	}
	return f.onRow(rows, min(f.index, len(rows)-1)), gone
}

// selected returns the row under the cursor.
func (f focus) selected(rows []row) (row, bool) {
	if f.row == "" {
		return row{}, false
	}
	i := slices.IndexFunc(rows, func(r row) bool { return r.key == f.row })
	if i < 0 {
		return row{}, false
	}
	return rows[i], true
}

func (f focus) onRow(rows []row, i int) focus {
	f.row, f.index = rows[i].key, i
	return f
}

// indexIn is the cursor's position in rows, its last known index when the
// key is missing.
func (f focus) indexIn(rows []row) int {
	if i := slices.IndexFunc(rows, func(r row) bool { return r.key == f.row }); i >= 0 {
		return i
	}
	return min(f.index, len(rows)-1)
}

// goneText is the status line for a row that left under the cursor:
// "pod shop/payments-0 is gone" for the key "pod/shop/payments-0".
func goneText(key string) string {
	kind, name, ok := strings.Cut(key, "/")
	if !ok {
		return key + " is gone"
	}
	return kind + " " + name + " is gone"
}
