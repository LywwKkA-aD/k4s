package services

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the service under the cursor, rows
// of a stale list included. There is none before the first list lands or
// when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	row := m.table.SelectedRow()
	if !m.loaded || row == nil {
		return views.ResourceRef{}, false
	}
	ns, name := serviceCoords(row, m.namespace)
	return views.ResourceRef{Kind: kindService, Namespace: ns, Name: name}, true
}
