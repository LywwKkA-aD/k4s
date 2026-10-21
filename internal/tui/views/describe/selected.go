package describe

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the object this view describes.
func (m Model) Selected() (views.ResourceRef, bool) {
	return views.ResourceRef{Kind: string(m.kind), Namespace: m.namespace, Name: m.name}, true
}
