package services

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const kindService = "service"

// row is the service under the cursor. Without one, cmd is the status
// that says why, so a row key never fails silently.
func (m Model) row() (ref views.ResourceRef, cmd tea.Cmd, ok bool) {
	switch {
	case m.client == nil:
		return ref, statusCmd(status.Warn, "no kubeconfig"), false
	case !m.loaded:
		return ref, statusCmd(status.Info, "services are still loading"), false
	case m.health.Failed():
		return ref, statusCmd(status.Warn, "services unavailable, press r"), false
	}
	row := m.table.SelectedRow()
	if row == nil {
		return ref, statusCmd(status.Warn, "no service selected"), false
	}
	ns, name := serviceCoords(row, m.namespace)
	return views.ResourceRef{Kind: kindService, Namespace: ns, Name: name}, nil, true
}

func (m Model) describe() tea.Cmd {
	ref, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	return func() tea.Msg {
		return views.DescribeRequestMsg{Kind: kindService, Namespace: ref.Namespace, Name: ref.Name}
	}
}

// forward is 'f'. The suggested remote port comes from the listed
// service, so nothing is resolved and there is nothing to guard: the
// port prompt the request opens takes the next key.
func (m Model) forward() tea.Cmd {
	ref, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	remote := uint16(0)
	for _, s := range m.raw {
		if s.Namespace == ref.Namespace && s.Name == ref.Name {
			remote = firstServicePort(s)
			break
		}
	}
	return func() tea.Msg {
		return views.ForwardRequestMsg{Kind: kindService, Namespace: ref.Namespace, Name: ref.Name, RemotePort: remote}
	}
}
