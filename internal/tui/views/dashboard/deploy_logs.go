package dashboard

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// l on a deployment row opens multi-pod logs (spec §4 panel 4, §5): the
// pods the deployment owns, resolved as the deployments view's l does,
// then the tail prompt or the container picker. The last l wins: an l on
// another row, or anything else l or Enter opens, drops the resolve in
// flight, and its reply opens the deployment l was pressed on, marked
// Detached when the cursor has left that row. A poll re-sorts the rows
// under a cursor kept by key, so it never changes the target.

const (
	actionLogs = "logs"
	// resolvingDelay is how long a resolve runs before the status row
	// says so: a quick answer should not flash a message.
	resolvingDelay = 150 * time.Millisecond
)

// logsResolvingMsg goes off resolvingDelay after the resolve under tok
// started.
type logsResolvingMsg struct{ tok async.Token }

// startDeployLogs resolves the pods of deployment k. A repeat l on the
// deployment being resolved starts nothing.
func (m Model) startDeployLogs(k dash.ObjectKey) (views.View, tea.Cmd) {
	tok := m.tok.Next()
	acts, ok := m.acts.Supersede(actionLogs, k.String(), tok)
	if !ok {
		return m, nil
	}
	m.tok, m.acts, m.logsFor = tok, acts, k
	c := m.client
	resolve := async.Run(tok, async.DefaultTimeout, func(ctx context.Context) (k8s.LogTarget, error) {
		return c.ResolveLogTarget(ctx, k8s.LogKindDeployment, k.Namespace, k.Name)
	})
	slow := m.clock.Tick(resolvingDelay, func(time.Time) tea.Msg { return logsResolvingMsg{tok: tok} })
	return m, tea.Batch(resolve, slow)
}

// dropLogs is m without the resolve in flight, whose reply is then
// dropped: the user asked for something else since.
func (m Model) dropLogs() Model {
	m.acts, _ = m.acts.End(actionLogs, m.tok)
	return m
}

func (m Model) onLogsResolving(msg logsResolvingMsg) tea.Cmd {
	if !m.acts.Accept(actionLogs, msg.tok) {
		return nil
	}
	return statusCmd(status.Info, "resolving logs for "+objectName(m.logsFor)+"…")
}

// onDeployLogs takes the reply of the resolve in flight and asks the root
// for the prompt it leads to, or says why there is nothing to stream.
func (m Model) onDeployLogs(msg async.Result[k8s.LogTarget]) (views.View, tea.Cmd) {
	acts, ok := m.acts.End(actionLogs, msg.Token)
	if !ok {
		return m, nil
	}
	m.acts = acts
	f := m.mem.focus
	r, on := f.selected(m.rows(f.panel))
	detached := !on || r.target != m.logsFor
	return m, deployLogsRequest(m.logsFor, msg.Value, msg.Err, detached)
}

// deployLogsRequest is the prompt request for the resolved target t of
// deployment k, or the status that says why there is none.
func deployLogsRequest(k dash.ObjectKey, t k8s.LogTarget, err error, detached bool) tea.Cmd {
	name := objectName(k)
	switch {
	case apierrors.IsNotFound(err):
		return statusCmd(status.Warn, goneObject(k))
	case errors.Is(err, k8s.ErrNoSelector):
		return statusCmd(status.Warn, name+" has no selector")
	case err != nil:
		return statusCmd(status.Error, "logs of "+name+": "+stale.Describe(err))
	case len(t.Pods) == 0 && t.Replicas == 0:
		return statusCmd(status.Warn, name+" has 0 replicas")
	case len(t.Pods) == 0:
		text := name + ": 0 running pods"
		if note := t.SkipNote(); note != "" {
			text += " · " + note
		}
		return statusCmd(status.Warn, text)
	}
	ref := views.ResourceRef{Kind: "deployment", Namespace: k.Namespace, Name: k.Name}
	req := views.LogsPromptFor(ref, t, detached)
	prompt := func() tea.Msg { return req }
	if !t.OwnersGuessed {
		return prompt
	}
	return tea.Batch(prompt, statusCmd(status.Info, "owners guessed: kubectl auth can-i list replicasets -n "+clean(k.Namespace)))
}

// objectName names k in a status: "deploy shop/payments-worker".
func objectName(k dash.ObjectKey) string {
	return change{obj: k}.object()
}
