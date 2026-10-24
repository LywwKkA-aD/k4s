package services

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const kindService = "service"

// row is the service under the cursor. Without one, cmd is the status
// that says why, so a row key never fails silently.
func (m Model) row() (svc k8s.Service, cmd tea.Cmd, ok bool) {
	switch {
	case m.client == nil:
		return svc, statusCmd(status.Warn, "no kubeconfig"), false
	case !m.loaded:
		return svc, statusCmd(status.Info, "services are still loading"), false
	case m.health.Failed():
		return svc, statusCmd(status.Warn, "services unavailable, press r"), false
	}
	if svc, ok = m.cursorService(); !ok {
		return svc, statusCmd(status.Warn, "no service selected"), false
	}
	return svc, nil, true
}

func (m Model) describe() tea.Cmd {
	svc, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	return func() tea.Msg {
		return views.DescribeRequestMsg{Kind: kindService, Namespace: svc.Namespace, Name: svc.Name}
	}
}

// forward is 'f'. The suggested remote port comes from the listed
// service, so nothing is resolved and there is nothing to guard: the
// port prompt the request opens takes the next key.
func (m Model) forward() tea.Cmd {
	svc, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	remote := firstServicePort(svc)
	return func() tea.Msg {
		return views.ForwardRequestMsg{Kind: kindService, Namespace: svc.Namespace, Name: svc.Name, RemotePort: remote}
	}
}
