package dashboard

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the object of the row under the
// cursor, so o opens its YAML. That is a row Enter describes: a pod,
// wherever it is listed, or a node or workload, placed in its namespace.
// The banner, a panel, the node chart, a summary line, a row of no object
// and one whose object is gone have none; the root then says there is no
// row.
func (m Model) Selected() (views.ResourceRef, bool) {
	f := m.mem.focus
	r, ok := f.selected(m.rows(f.panel))
	if !ok || !describable(r) {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: r.target.Kind, Namespace: r.target.Namespace, Name: r.target.Name}, true
}
