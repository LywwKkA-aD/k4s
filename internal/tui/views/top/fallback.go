package top

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
)

// fallBack answers RBAC refusing the pod metrics of every namespace by
// asking for the kubeconfig context's namespace instead, as kubectl top
// would without -A; ok is false when err is something else or the
// context names no namespace. Nodes are cluster-scoped and never fall
// back. Mode, sort and watch are kept, and the fetch goes out at once.
func (m Model) fallBack(err error) (Model, tea.Cmd, bool) {
	if m.namespace != "" || m.mode != modePods || m.client == nil {
		return m, nil, false
	}
	ns := m.client.ContextNamespace
	say, ok := stale.Fallback(err, ns)
	if !ok {
		return m, nil, false
	}
	m.namespace = ns
	pending := m.pending
	m = m.withMode(m.mode)
	m.pending = pending
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	m.mint = m.mint.Next()
	m.poll = m.poll.Restart(m.mint)
	return m, tea.Batch(say, m.fetch()), true
}
