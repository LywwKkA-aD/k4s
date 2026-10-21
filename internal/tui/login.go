package tui

import (
	"context"
	"os/exec"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// loginProbeTimeout bounds the /version call that checks a context's
// credentials; a slow plugin that does log in takes seconds.
const loginProbeTimeout = 15 * time.Second

// The login flow after :ctx (M4.7). A context whose credential plugin
// needs a terminal fails under the TUI, which owns it, and client-go keeps
// it from prompting there (StdinUnavailable). So the first call after the
// switch is a /version probe; when it fails for want of credentials, a
// confirm offers to run `kubectl version` for that context in the
// terminal, which lets the plugin prompt and cache a token, and then
// probes again. A plugin that keeps no token it can hand on fails that
// second probe too, and k4s says so instead of asking again.
//
// Each step is an execDoneMsg carrying the context it was for, so a step
// that arrives after another switch is dropped.

// loginProbe is the probe for c, nil for a context without a credential
// plugin: tokens and certificates need no terminal.
func loginProbe(c *k8s.Client, afterLogin bool) tea.Cmd {
	if c == nil || c.RestConfig == nil || c.RestConfig.ExecProvider == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), loginProbeTimeout)
		defer cancel()
		_, err := c.ServerVersion(ctx)
		return loginProbeMsg{context: c.Context, afterLogin: afterLogin, err: err}
	}
}

// loginProbeMsg is the answer of a probe of context; afterLogin marks the
// one that follows `kubectl version`.
type loginProbeMsg struct {
	context    string
	afterLogin bool
	err        error
}

func (p loginProbeMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if !m.onContext(p.context) {
		return m, nil
	}
	d := k8s.Classify(p.err)
	needsLogin := d.Kind == k8s.KindUnauthorized || d.Kind == k8s.KindLogin
	switch {
	case p.err == nil && p.afterLogin:
		m, said := m.setStatus(status.OK, "logged in to "+p.context)
		m, reload := m.reloadView()
		return m, tea.Batch(said, reload)
	case p.err == nil:
		return m, nil
	case p.afterLogin && needsLogin:
		return m.setError("context " + p.context + ": logged in, but the credential plugin still fails without a terminal: " +
			"it keeps no token k4s can reuse; turn on its token cache")
	case p.afterLogin:
		return m.setError("context " + p.context + ": " + stale.Describe(p.err))
	case !needsLogin:
		// The views say what is wrong with the cluster.
		return m, nil
	case m.overlay != nil:
		return m.setStatus(status.Warn, m.loginAdvice(p.context))
	}
	m.overlay = overlay.NewLogin(p.context, kubectl.New("version"),
		loginAnswerMsg{context: p.context, yes: true}, loginAnswerMsg{context: p.context}, m.now)
	return m, nil
}

// loginAnswerMsg is the answer to the confirm for context.
type loginAnswerMsg struct {
	context string
	yes     bool
}

// apply hands the terminal to `kubectl version` for the context, through
// the runner and decorated like the footer: the plugin runs with a
// terminal and can prompt. A skipped login says how to do it by hand.
func (a loginAnswerMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if !m.onContext(a.context) {
		return m, nil
	}
	if !a.yes {
		return m.setStatus(status.Warn, m.loginAdvice(a.context))
	}
	c, err := m.loginCommand()
	if err != nil {
		return m.setError("login: " + err.Error())
	}
	ctx := a.context
	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return loginDoneMsg{context: ctx, err: err}
	})
}

// loginCommand is `kubectl version` for the active context as the runner
// starts it.
func (m Model) loginCommand() (*exec.Cmd, error) {
	return m.runner.Command(m.decorate(kubectl.New("version")))
}

// loginAdvice is how to log in by hand.
func (m Model) loginAdvice(ctx string) string {
	return "context " + ctx + " needs a login: run " + m.decorate(kubectl.New("version")).String() +
		" in a terminal, then press r"
}

// loginDoneMsg is `kubectl version` back from the terminal.
type loginDoneMsg struct {
	context string
	err     error
}

// apply probes again once kubectl logged in.
func (d loginDoneMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if !m.onContext(d.context) {
		return m, nil
	}
	if d.err != nil {
		return m.setError("login: " + d.err.Error() + " · " + m.loginAdvice(d.context))
	}
	m, said := m.setStatus(status.Info, "connecting to "+d.context+"…")
	return m, tea.Batch(said, loginProbe(m.client, true))
}

// onContext reports whether the active client is still on context.
func (m Model) onContext(context string) bool {
	return m.client != nil && m.client.Context == context
}

// reloadView rebuilds the active view as it is, so it lists again with
// the credentials that now work.
func (m Model) reloadView() (Model, tea.Cmd) {
	title := m.current.Title()
	if !isRebuildableView(title) {
		return m, nil
	}
	st := stateOf(m.current)
	m = m.replaceView(title)
	m, restored := m.restore(st)
	return m, tea.Batch(restored, m.relayoutCmd())
}
