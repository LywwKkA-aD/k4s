package logs

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// opening is a request for the previous instance's log in flight. The
// view goes on streaming the current log until it is answered, so a
// container that never restarted costs a status line, not the stream.
type opening struct {
	gen    int
	cancel context.CancelFunc
}

func (o opening) busy() bool { return o.cancel != nil }

func (o opening) stop() {
	if o.cancel != nil {
		o.cancel()
	}
}

// previousOpenedMsg answers an opening: the session reading the previous
// instance's log, or why it could not be opened.
type previousOpenedMsg struct {
	gen  int
	sess session
	err  error
}

// togglePrevious is p: a single pod's stream switches to the previous
// instance of its container and back.
func (m Model) togglePrevious() (Model, tea.Cmd) {
	switch {
	case m.src == nil:
		return m, statusCmd(status.Warn, "no kubeconfig")
	case len(m.pods) != 1:
		return m, statusCmd(status.Warn, "previous logs are per pod: open the logs of one pod")
	case m.opening.busy():
		return m, statusCmd(status.Info, "opening the previous instance…")
	case m.previous:
		m.previous = false
		m.lastGen++
		return m.restart(startSession(m.lastGen, m.spec()))
	}
	m.lastGen++
	ctx, cancel := context.WithCancel(context.Background())
	m.opening = opening{gen: m.lastGen, cancel: cancel}
	return m, openPrevious(ctx, cancel, m.lastGen, m.spec())
}

// openPrevious opens the previous instance's log and starts reading it.
// The stream is opened before the view gives up the current one, so an
// error leaves the view as it was.
func openPrevious(ctx context.Context, cancel context.CancelFunc, gen int, spec streamSpec) tea.Cmd {
	pod, opts := spec.pods[0], spec.options()
	opts.Previous = true
	return func() tea.Msg {
		stream, err := spec.src.StreamLogs(ctx, spec.namespace, pod, opts)
		if err != nil {
			cancel()
			return previousOpenedMsg{gen: gen, err: err}
		}
		events := make(chan logEvent, eventBufferSize)
		go func() {
			defer close(events)
			readOnePod(ctx, stream, pod, events)
		}()
		return previousOpenedMsg{gen: gen, sess: session{gen: gen, events: events, cancel: cancel}}
	}
}

func (m Model) onPreviousOpened(msg previousOpenedMsg) (views.View, tea.Cmd) {
	if !m.opening.busy() || msg.gen != m.opening.gen {
		msg.sess.stop()
		return m, nil
	}
	m.opening = opening{}
	if msg.err != nil {
		return m, previousFailed(m.namespace, m.pods[0], m.container, msg.err)
	}
	m.previous = true
	return m.restart(msg.sess)
}

// restart moves the view onto s. The old streams stop and the buffer
// starts empty: the log it showed is not the one s reads. The search
// stays and applies to the new lines.
func (m Model) restart(s session) (Model, tea.Cmd) {
	m.sess.stop()
	m.sess = s
	m.autoFollow = true
	m.clearLogs()
	return m, waitOrDrainCmd(s)
}

// previousFailed says why the previous instance's log could not be
// opened, in words a user can act on.
func previousFailed(ns, pod, container string, err error) tea.Cmd {
	text := sanitize.Line(apiMessage(err))
	switch {
	case apierrors.IsBadRequest(err) && strings.Contains(text, "previous terminated container"):
		return statusCmd(status.Warn, noPrevious(pod, container))
	case apierrors.IsNotFound(err):
		return statusCmd(status.Warn, "pod "+pod+" is gone")
	case apierrors.IsForbidden(err):
		return statusCmd(status.Error, "forbidden: cannot read the logs of pod "+pod+" in "+ns+
			" (kubectl auth can-i get pods --subresource=log -n "+ns+")")
	}
	return statusCmd(status.Error, "previous logs of pod "+pod+": "+text)
}

// apiMessage is the API server's own message when err carries a status,
// without the request URL a transport error wraps it in.
func apiMessage(err error) string {
	var st apierrors.APIStatus
	if errors.As(err, &st) {
		return st.Status().Message
	}
	return err.Error()
}

func noPrevious(pod, container string) string {
	if container == "" {
		return "pod " + pod + " has no previous instance: its container has not restarted"
	}
	return "container " + container + " of pod " + pod + " has no previous instance: it has not restarted"
}

// previousCommand is c, a single pod's follow command, reading the
// previous instance instead: --previous and no -f, as that log has ended.
func previousCommand(c kubectl.Cmd) kubectl.Cmd {
	if c.IsZero() {
		return c
	}
	args := make([]string, 0, len(c.Args)+1)
	for _, a := range c.Args {
		if a != "-f" {
			args = append(args, a)
		}
	}
	out := kubectl.New(append(args, "--previous")...)
	out.Approx = c.Approx
	return out
}

func statusCmd(level status.Level, text string) tea.Cmd {
	msg := views.StatusMsg{Level: level, Text: text}
	return func() tea.Msg { return msg }
}
