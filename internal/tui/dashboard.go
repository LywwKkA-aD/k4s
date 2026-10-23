package tui

import (
	tea "charm.land/bubbletea/v2"
	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// newDashboard builds the dashboard in the active namespace, with what the
// last dashboard learned, the terminal's focus and colour profile as last
// reported and the session the footer decorates with, so commands printed
// in its body paste as the footer's do. When the root asks /version for the
// header, the dashboard takes that answer instead of asking again; the
// distribution the root detected words the dashboard's hints, as top's. A
// failed request is handed over too, so the dashboard shows the failure.
func (m Model) newDashboard() views.View {
	opts := []dashboard.Option{
		dashboard.WithNamespace(m.namespace),
		dashboard.WithMemory(m.dashMem),
		dashboard.WithBlurred(m.blurred),
		dashboard.WithColorProfile(m.profile),
		dashboard.WithSession(m.kubectlSession),
		dashboard.WithDistro(m.startup.Distro),
	}
	if m.ownsVersion() {
		opts = append(opts, dashboard.WithServerVersion(m.startup.ServerVersion), dashboard.WithServerVersionError(m.ident.versionErr))
	}
	return dashboard.New(m.client, m.loadErr, opts...)
}

// tellDashboard hands the server version, or err when the request for it
// failed, and the distribution detected to the dashboard on screen; one
// built later takes them from the session (see newDashboard).
func (m Model) tellDashboard(info *apiversion.Info, err error) (Model, tea.Cmd) {
	d, ok := m.current.(dashboard.Model)
	if !ok {
		return m, nil
	}
	var cmd tea.Cmd
	m.current, cmd = d.Update(dashboard.VersionMsg{Info: info, Distro: m.startup.Distro, Err: err})
	return m, cmd
}

// keepDashboard takes v's memory when v is a dashboard about to be
// replaced, so the next one shows its data and history at once. Unlike
// history entries it survives going home; :ctx drops it.
func (m Model) keepDashboard(v views.View) Model {
	if d, ok := v.(dashboard.Model); ok {
		m.dashMem = d.Memory()
	}
	return m
}

// onTerminalFocus remembers whether the terminal has focus, for dashboards
// built later, and hands the message to the active view: the dashboard
// polls slower while blurred, and holds still.
func (m Model) onTerminalFocus(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, m.blurred = msg.(tea.BlurMsg)
	return m.forwardToView(msg)
}

// onColorProfile remembers the profile the program detected, which it
// reports once at start, for dashboards built later, and hands it to the
// active view.
func (m Model) onColorProfile(msg tea.ColorProfileMsg) (tea.Model, tea.Cmd) {
	m.profile = msg.Profile
	return m.forwardToView(msg)
}
