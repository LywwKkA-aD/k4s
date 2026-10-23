package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	apiversion "k8s.io/apimachinery/pkg/version"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// versionTimeout bounds each of the header's /version requests.
const versionTimeout = 5 * time.Second

// versionRetries are the waits before the header asks again for a version
// it could not get, one per retry; the last repeats until the server
// answers. Nothing else re-asks after an outage, so a header that stopped
// would stay blank for the rest of the session.
var versionRetries = [...]time.Duration{15 * time.Second, 30 * time.Second, time.Minute}

// versionMsg is the reply to the header's /version request; attempt counts
// the asks before it, 0 for the first.
//
// It applies itself (see selfApplyingMsg) because a failed reply arms a retry,
// and the router keeps the Cmd of a self-applying message only.
type versionMsg struct {
	res     async.Result[*apiversion.Info]
	attempt int
}

// versionRetryMsg is the end of the wait before ask number attempt.
type versionRetryMsg struct {
	token   async.Token
	attempt int
}

// fetchVersion asks the server for its version, after the start and after
// :ctx: it names the distribution and the release in the header.
// Only certificate and token contexts ask here. An exec or auth-provider
// context got its version from the preflight, and running its plugin
// under the TUI could prompt where nobody can answer.
func (m Model) fetchVersion() tea.Cmd {
	return m.askVersion(0)
}

// ownsVersion reports whether the session knows the active client's server
// version or asks it (see fetchVersion), so the dashboard need not ask.
func (m Model) ownsVersion() bool {
	return m.startup.ServerVersion != nil || (m.client != nil && !m.client.NeedsLogin())
}

// askVersion is ask number attempt under the client's token, or nil when
// there is nothing to ask (see fetchVersion).
func (m Model) askVersion(attempt int) tea.Cmd {
	c := m.client
	if c == nil || c.NeedsLogin() || m.startup.ServerVersion != nil {
		return nil
	}
	run := async.Run(m.ident.version, versionTimeout, c.ServerVersion)
	return func() tea.Msg {
		res, _ := run().(async.Result[*apiversion.Info])
		return versionMsg{res: res, attempt: attempt}
	}
}

// versionRetryAfter is how long the header waits after a failed ask number
// attempt before it asks again; false for an attempt that was never made.
func versionRetryAfter(attempt int) (time.Duration, bool) {
	if attempt < 0 {
		return 0, false
	}
	return versionRetries[min(attempt, len(versionRetries)-1)], true
}

// versionRetryable reports whether asking again could get the version: not
// once the server has answered and refused (401, 403, 404, a login).
func versionRetryable(err error) bool {
	switch k8s.Classify(err).Kind {
	case k8s.KindUnauthorized, k8s.KindForbidden, k8s.KindLogin, k8s.KindNotFound:
		return false
	}
	return true
}

// apply records the active client's server version and hands it to the
// dashboard on screen. A failure leaves the header as it is and goes to
// the dashboard, which shows it on its version row as it shows a failure
// of its own requests (a denial with its can-i probe). Unless the server
// refused for good, the header asks again after a wait that grows to a
// minute, for as long as the server stays away, so a cluster that was down
// at the start still gets named. A reply for a client k4s has switched
// away from is dropped.
func (v versionMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if m.client == nil || !m.ident.version.Accept(v.res.Token) {
		return m, nil
	}
	if info := v.res.Value; v.res.Err == nil && info != nil {
		m.startup.ServerVersion = info
		m.startup.Distro = cluster.Detect(info.GitVersion, m.client.Context, nil)
		m.ident.versionErr = nil
		return m.tellDashboard(info, nil)
	}
	var told tea.Cmd
	if v.res.Err != nil {
		m.ident.versionErr = v.res.Err
		m, told = m.tellDashboard(nil, v.res.Err)
	}
	wait, ok := versionRetryAfter(v.attempt)
	if !ok || !versionRetryable(v.res.Err) {
		return m, told
	}
	retry := versionRetryMsg{token: v.res.Token, attempt: v.attempt + 1}
	return m, tea.Batch(told, tea.Tick(wait, func(time.Time) tea.Msg { return retry }))
}

// apply asks again unless the client changed or the version is known.
func (r versionRetryMsg) apply(m Model) (tea.Model, tea.Cmd) {
	if !m.ident.version.Accept(r.token) {
		return m, nil
	}
	return m, m.askVersion(r.attempt)
}
