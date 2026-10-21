package pods

import "github.com/LywwKkA-aD/k4s/internal/tui/views"

// Selected implements views.Selection: the pod under the cursor, rows of
// a stale list included. There is none before the first list lands or
// when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	p, ok := m.cursorPod()
	if !ok || p.Name == "" {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: "pod", Namespace: p.Namespace, Name: p.Name}, true
}
