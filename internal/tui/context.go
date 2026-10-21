package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// onContextSelected swaps the active k8s.Client to the chosen context,
// read from the same kubeconfig files. It drops the navigation history
// and every saved view state (they reference resources that may not
// exist in the new cluster), starts in the new context's namespace,
// rebuilds the kubectl session and lands on the dashboard. Failures
// surface in the status row and the previous client stays active.
func (m Model) onContextSelected(msg views.ContextSelectedMsg) (tea.Model, tea.Cmd) {
	client, err := m.switchClient(msg.Name)
	if err != nil {
		return m.setError("context: " + err.Error())
	}
	if m.current != nil {
		_ = m.current.Close()
	}
	m = m.withClient(client)
	m.loadErr = nil
	m.history = nil
	m.status = m.status.Clear()
	m.startup = startup.Session{Source: m.startup.Source, Distro: cluster.Detect("", client.Context, nil)}
	m, errCmd := m.resetForwards(client)
	m.current = dashboard.New(m.client, nil)
	return m, tea.Batch(m.relayoutCmd(), errCmd)
}

// switchClient loads context name from the session's kubeconfig: through
// the active client, else from the Source startup found.
func (m Model) switchClient(name string) (*k8s.Client, error) {
	if m.client != nil {
		return m.client.SwitchContext(name)
	}
	return k8s.Load(m.startup.Source, name)
}

// resetForwards is the one place a context switch touches port-forwards:
// it stops the old cluster's sessions, so their local ports are freed and
// no SPDY goroutine outlives its cluster, then opens the state for the new
// client. StopAll blocks until the sessions are down; a non-blocking stop
// replaces it here and nowhere else.
func (m Model) resetForwards(client *k8s.Client) (Model, tea.Cmd) {
	if m.forwardMgr != nil {
		_ = m.forwardMgr.StopAll()
	}
	mgr, err := forwards.NewManager(client)
	if err != nil {
		m.forwardMgr = nil
		return m.setError("forwards: " + err.Error())
	}
	m.forwardMgr = mgr
	return m, nil
}
