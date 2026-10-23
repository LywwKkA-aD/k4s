package pods

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// fallBack answers RBAC refusing the pods of every namespace by listing
// the kubeconfig context's namespace instead, as kubectl would without
// -A; ok is false when err is something else or the context names no
// namespace. The view becomes that namespace's list, filter and watch
// kept, and its first list goes out at once.
func (m Model) fallBack(err error) (Model, tea.Cmd, bool) {
	if m.namespace != "" || m.client == nil {
		return m, nil, false
	}
	ns := m.client.ContextNamespace
	say, ok := stale.Fallback(err, ns)
	if !ok {
		return m, nil, false
	}
	m.namespace = ns
	m.raw, m.shown, m.loaded = nil, nil, false
	m.health = m.health.Reset()
	m.table = newTable(layout(m.width, false).columns())
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	m.tok = m.tok.Next()
	m.poll = m.poll.Restart(m.tok)
	return m, tea.Batch(say, m.list(k8s.FromCache()), views.NamespaceFallback(m.Title(), ns)), true
}
