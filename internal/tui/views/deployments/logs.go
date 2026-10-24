package deployments

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	actionLogs = "logs"
	// resolvingDelay is how long a logs resolve runs before the status row
	// says so: a quick answer should not flash a message.
	resolvingDelay = 150 * time.Millisecond
)

// resolvingMsg goes off resolvingDelay after the logs resolve under tok
// started.
type resolvingMsg struct {
	tok async.Token
}

// startLogs resolves the pods behind d. The last l wins: an l on another
// row supersedes the resolve in flight, whose reply is then dropped, and a
// repeat l on the row being resolved starts nothing.
func (m Model) startLogs(d k8s.Deployment) (Model, tea.Cmd) {
	tok := m.tok.Next()
	acts, ok := m.acts.Supersede(actionLogs, views.RowKey(d.Namespace, d.Name), tok)
	if !ok {
		return m, nil
	}
	m.tok, m.acts = tok, acts
	m.logsFor = views.ResourceRef{Kind: kind, Namespace: d.Namespace, Name: d.Name}
	c, ns, name := m.client, d.Namespace, d.Name
	resolve := async.Run(tok, async.DefaultTimeout, func(ctx context.Context) (k8s.LogTarget, error) {
		return c.ResolveLogTarget(ctx, k8s.LogKindDeployment, ns, name)
	})
	slow := m.clock.Tick(resolvingDelay, func(time.Time) tea.Msg { return resolvingMsg{tok: tok} })
	return m, tea.Batch(resolve, slow)
}

func (m Model) onResolving(msg resolvingMsg) tea.Cmd {
	if !m.acts.Accept(actionLogs, msg.tok) {
		return nil
	}
	r := m.logsFor
	return statusCmd(status.Info, "resolving logs for "+r.Kind+" "+views.RowKey(r.Namespace, r.Name)+"…")
}

// onLogTarget takes the reply of the resolve in flight and asks the root
// for the prompt it leads to. The prompt opens even when the cursor has
// left the row meanwhile, marked Detached.
func (m Model) onLogTarget(msg async.Result[k8s.LogTarget]) (Model, tea.Cmd) {
	acts, ok := m.acts.End(actionLogs, msg.Token)
	if !ok {
		return m, nil
	}
	m.acts = acts
	at, _ := m.cursorKey()
	r := m.logsFor
	detached := at != views.RowKey(r.Namespace, r.Name)
	return m, logsRequest(r, msg.Value, msg.Err, detached)
}

// logsRequest is the prompt request for a resolved target t of r, or the
// status that says why there is nothing to stream.
func logsRequest(r views.ResourceRef, t k8s.LogTarget, err error, detached bool) tea.Cmd {
	switch {
	case errors.Is(err, k8s.ErrNoSelector):
		return statusCmd(status.Warn, kind+" "+r.Name+" has no selector")
	case err != nil:
		return statusCmd(status.Error, kind+" "+r.Name+": "+stale.Describe(err))
	case len(t.Pods) == 0:
		return statusCmd(status.Warn, noRunningPods(r.Name, t))
	}
	req := request(views.LogsPromptFor(r, t, detached))
	if !t.OwnersGuessed {
		return req
	}
	return tea.Batch(req, statusCmd(status.Info, "owners guessed: kubectl auth can-i list replicasets -n "+r.Namespace))
}

func noRunningPods(name string, t k8s.LogTarget) string {
	text := kind + " " + name + ": 0 running pods"
	if note := t.SkipNote(); note != "" {
		return text + " · " + note
	}
	return text
}
