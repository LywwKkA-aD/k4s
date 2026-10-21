package deployments

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// fallBack answers RBAC refusing the deployments of every namespace by
// listing the kubeconfig context's namespace instead, as kubectl would
// without -A; ok is false when err is something else or the context names
// no namespace. Filter and watch are kept, and the first list goes out at
// once.
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
	m.table.SetRows(nil)
	m.table.SetColumns(tableColumns(false))
	styles.FitTable(&m.table)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	m.tok = m.tok.Next()
	m.poll = m.poll.Restart(m.tok)
	return m, tea.Batch(say, m.list(k8s.FromCache())), true
}
