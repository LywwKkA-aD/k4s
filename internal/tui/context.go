package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// onContextSelected swaps the active k8s.Client to the chosen context,
// read from the same kubeconfig files. It drops the navigation history,
// every saved view state and the namespaces used so far (they reference
// resources that may not exist in the new cluster), starts in the new
// context's namespace,
// rebuilds the kubectl session and lands on the dashboard. Failures
// surface in the status row and the previous client stays active. A
// context with a credential plugin is probed for a login (login.go).
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
	m.history, m.usedNamespaces = nil, nil
	m.status = m.status.Clear()
	m.startup = startup.Session{Source: m.startup.Source, Distro: cluster.Detect("", client.Context, nil)}
	m, errCmd := m.resetForwards(client)
	m.dashMem = dashboard.Memory{}
	m.current = m.newDashboard()
	return m, tea.Batch(m.relayoutCmd(), errCmd, loginProbe(client, false), m.fetchVersion())
}

// switchClient loads context name from the session's kubeconfig: through
// the active client, else from the Source startup found.
func (m Model) switchClient(name string) (*k8s.Client, error) {
	if m.client != nil {
		return m.client.SwitchContext(name)
	}
	return k8s.Load(m.startup.Source, name)
}

// resetForwards is the one place a context switch touches port-forwards.
// It closes the old cluster's sessions at once, so their local ports are
// freed and no SPDY goroutine outlives its cluster, and returns the wait
// for them as a Cmd: a hung tunnel must not freeze the switch. The new
// client's Manager is built After that stop and refuses to register or
// start a forward until the old sessions are down or timed out, since a
// new forward could otherwise lose its local port to an old one. A state
// file the new Manager could not read is reported over the stop, and
// again in the stop's own report once it ends, which replaces it.
func (m Model) resetForwards(client *k8s.Client) (Model, tea.Cmd) {
	var stop *forwards.Stopping
	if m.forwardMgr != nil {
		stop = m.forwardMgr.BeginStopAll()
	}
	mgr, err := forwards.NewManager(client, forwards.After(stop))
	m.forwardMgr = mgr
	notice := forwardsNotice(mgr)
	stopping, waitCmd := stopReport(stop, notice)
	var statusCmd tea.Cmd
	switch {
	case err != nil:
		m, statusCmd = m.setError("forwards: " + err.Error())
	case notice != "":
		m, statusCmd = m.setStatus(status.Warn, notice)
	case waitCmd != nil:
		m, statusCmd = m.setStatus(status.Info, "stopping "+stopping+"…")
	}
	return m, tea.Batch(statusCmd, waitCmd)
}

// stopReport is how many forwards stop is stopping and the Cmd that waits
// for it and says how it went; "" and nil when it stops none. The status
// row holds one message, so a notice shown while the stop ran rides on
// the report, which would otherwise replace it unread.
func stopReport(stop *forwards.Stopping, notice string) (string, tea.Cmd) {
	if stop == nil || stop.Count() == 0 {
		return "", nil
	}
	stopping := fmt.Sprintf("%d forwards", stop.Count())
	if stop.Count() == 1 {
		stopping = "1 forward"
	}
	return stopping, func() tea.Msg {
		report := views.StatusMsg{Level: status.OK, Text: "stopped " + stopping}
		if err := stop.Wait(); err != nil {
			report = views.StatusMsg{Level: status.Warn, Text: "forwards: " + err.Error()}
		}
		if notice == "" {
			return report
		}
		return views.StatusMsg{Level: status.Warn, Text: report.Text + " · " + notice}
	}
}
