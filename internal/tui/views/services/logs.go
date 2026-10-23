package services

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const actLogs = "logs"

// logsNoticeDelay is how long a logs resolve may run before the status
// line says what it is waiting for; a quick one says nothing.
const logsNoticeDelay = 150 * time.Millisecond

// logsSlowMsg goes off logsNoticeDelay after the logs resolve under tok
// started.
type logsSlowMsg struct{ tok async.Token }

// logs is 'l': resolve the pods behind the service under the cursor. The
// last request wins: l on another row drops the answer to the one before,
// and l again on the row being resolved is ignored.
func (m Model) logs() (Model, tea.Cmd) {
	ref, cmd, ok := m.row()
	if !ok {
		return m, cmd
	}
	tok := m.tok.Next()
	acts, ok := m.acts.Supersede(actLogs, views.RowKey(ref.Namespace, ref.Name), tok)
	if !ok {
		return m, nil
	}
	m.tok, m.acts, m.logsFor = tok, acts, ref
	c := m.client
	resolve := async.Run(tok, async.DefaultTimeout, func(ctx context.Context) (k8s.LogTarget, error) {
		return c.ResolveLogTarget(ctx, k8s.LogKindService, ref.Namespace, ref.Name)
	})
	notice := m.clock.Tick(logsNoticeDelay, func(time.Time) tea.Msg { return logsSlowMsg{tok: tok} })
	return m, tea.Batch(resolve, notice)
}

func (m Model) onLogsSlow(msg logsSlowMsg) tea.Cmd {
	if !m.acts.Accept(actLogs, msg.tok) {
		return nil
	}
	r := m.logsFor
	return statusCmd(status.Info, "resolving logs for service "+r.Namespace+"/"+r.Name+"…")
}

// onLogTarget opens the prompt for a resolved service, or says why not.
// The cursor may have left the row meanwhile: the prompt opens anyway,
// marked Detached, and Esc there cancels it.
func (m Model) onLogTarget(msg async.Result[k8s.LogTarget]) (Model, tea.Cmd) {
	var ok bool
	if m.acts, ok = m.acts.End(actLogs, msg.Token); !ok {
		return m, nil
	}
	ref := m.logsFor
	m.logsFor = views.ResourceRef{}
	if msg.Err != nil {
		return m, logsFailed(ref, msg.Err)
	}
	t := msg.Value
	if len(t.Pods) == 0 {
		text := "no pods back service " + ref.Name
		if note := t.SkipNote(); note != "" {
			text += "; " + note
		}
		return m, statusCmd(status.Warn, text)
	}
	detached := m.selectedKey() != views.RowKey(ref.Namespace, ref.Name)
	prompt := logsPrompt(ref, t, detached)
	// The owner's call: finished pods are skipped, and the note says the
	// footer's `kubectl logs -l` would still stream them.
	if note := t.SkipNote(); note != "" {
		return m, tea.Batch(statusCmd(status.Info, "≈ "+note), prompt)
	}
	return m, prompt
}

func logsFailed(ref views.ResourceRef, err error) tea.Cmd {
	switch {
	case errors.Is(err, k8s.ErrNoSelector):
		return statusCmd(status.Warn, "service "+ref.Name+" has no selector")
	case apierrors.IsNotFound(err):
		return statusCmd(status.Warn, "service "+ref.Name+" is gone, press r")
	}
	return statusCmd(status.Error, "logs for service "+ref.Name+": "+stale.Describe(err))
}

// logsPrompt asks the root for the container picker when the pods run
// several containers, init containers and native sidecars counted, else
// for the tail prompt.
func logsPrompt(ref views.ResourceRef, t k8s.LogTarget, detached bool) tea.Cmd {
	pods := t.PodNames()
	if len(t.Containers) > 1 {
		return func() tea.Msg {
			return views.ContainerPromptRequestMsg{
				Namespace: ref.Namespace, Pods: pods, Containers: t.Containers, Init: t.Init, NextKind: actLogs,
				Target: ref, Logs: t, Detached: detached,
			}
		}
	}
	container := ""
	if len(t.Containers) == 1 {
		container = t.Containers[0]
	}
	return func() tea.Msg {
		return views.TailPromptRequestMsg{
			Namespace: ref.Namespace, Pods: pods, Container: container,
			Target: ref, Logs: t, Detached: detached,
		}
	}
}
