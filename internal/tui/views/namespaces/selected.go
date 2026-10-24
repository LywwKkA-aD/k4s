package namespaces

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the namespace under the cursor,
// which has no namespace of its own. The <all> row is no object, and
// there is none before the first list lands.
func (m Model) Selected() (views.ResourceRef, bool) {
	name, ok := m.cursorName()
	if !m.loaded || !ok || m.table.Cursor() == 0 {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: "namespace", Name: name}, true
}
