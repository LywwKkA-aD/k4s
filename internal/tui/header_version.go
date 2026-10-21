package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// versionTimeout bounds the header's /version request.
const versionTimeout = 5 * time.Second

// versionMsg is the reply to the header's /version request.
type versionMsg struct {
	res async.Result[*apiversion.Info]
}

// fetchVersion asks the server for its version once, after the start and
// after :ctx: it names the distribution and the release in the header.
// Only certificate and token contexts ask here. An exec or auth-provider
// context got its version from the preflight, and running its plugin
// under the TUI could prompt where nobody can answer.
func (m Model) fetchVersion() tea.Cmd {
	c := m.client
	if c == nil || c.NeedsLogin() || m.startup.ServerVersion != nil {
		return nil
	}
	run := async.Run(m.ident.version, versionTimeout, c.ServerVersion)
	return func() tea.Msg {
		res, _ := run().(async.Result[*apiversion.Info])
		return versionMsg{res: res}
	}
}

// onVersion records the active client's server version. A failure changes
// nothing, since the dashboard reports an unreachable server, and a reply
// for a client k4s has switched away from is dropped.
func (m Model) onVersion(msg versionMsg) Model {
	info := msg.res.Value
	if m.client == nil || !m.ident.version.Accept(msg.res.Token) || msg.res.Err != nil || info == nil {
		return m
	}
	m.startup.ServerVersion = info
	m.startup.Distro = cluster.Detect(info.GitVersion, m.client.Context, nil)
	return m
}
