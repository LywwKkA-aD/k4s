package services

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the service under the cursor, rows
// of a stale list included. There is none before the first list lands or
// when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	s, ok := m.cursorService()
	if !m.loaded || !ok {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: kindService, Namespace: s.Namespace, Name: s.Name}, true
}
