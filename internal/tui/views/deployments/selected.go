package deployments

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the deployment under the cursor,
// rows of a stale list included. There is none before the first list
// lands or when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	d, ok := m.selected()
	if !ok || d.Name == "" {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: kind, Namespace: d.Namespace, Name: d.Name}, true
}
