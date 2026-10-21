package namespaces

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the namespace under the cursor,
// which has no namespace of its own. The <all> row is no object, and
// there is none before the first list lands.
func (m Model) Selected() (views.ResourceRef, bool) {
	row := m.table.SelectedRow()
	if !m.loaded || row == nil || row[0] == allRowName {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: "namespace", Name: row[0]}, true
}
