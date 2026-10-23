package logs

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// opening is a request for the previous instance's log in flight. The
// view goes on streaming the current log until it is answered, so a
// container that never restarted costs a status line, not the stream.
type opening struct {
	tok    async.Token
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
	tok  async.Token
	sess session
	err  error
}

// togglePrevious is p: a single pod's stream switches to the previous
// instance of its container and back. The previous instance's log has
// ended, so it is read once and never reconnected, and a following view
// stops re-resolving while it shows.
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
		m.sess.stop()
		m.autoFollow = true
		m.clearLogs()
		m = m.begin(m.pods)
		return m, tea.Batch(waitOrDrainCmd(m.sess), m.startup)
	}
	m.mint = m.mint.Next()
	ctx, cancel := context.WithCancel(context.Background())
	m.opening = opening{tok: m.mint, cancel: cancel}
	pod := m.pods[0]
	container := m.container
	if s, ok := m.streams.pods[pod]; ok {
		container = s.container
	}
	opts := k8s.LogOptions{Container: container, Tail: m.tail, Previous: true, Timestamps: true}
	return m, openPrevious(ctx, cancel, m.mint, m.sessionOf(pod), opts)
}

// WithPrevious opens the view on the previous instance of the pod's
// container (kubectl logs --previous), as p would right after it opened:
// the current log streams until the previous one is answered, and stays
// with a status saying why when there is none.
func WithPrevious() Option {
	return func(m *Model) { m.askPrevious = true }
}

// startPrevious asks for the previous instance when the view was built
// WithPrevious; the request goes out with Init.
func (m Model) startPrevious() Model {
	if !m.askPrevious {
		return m
	}
	m.askPrevious = false
	next, cmd := m.togglePrevious()
	next.startup = tea.Batch(next.startup, cmd)
	return next
}

// sessionOf is what a session for pod's previous log needs from the view.
func (m Model) sessionOf(pod string) previousSpec {
	return previousSpec{src: m.src, namespace: m.namespace, pod: pod, crash: m.crash}
}

// previousSpec is where the previous instance's log is read from.
type previousSpec struct {
	src       streamer
	namespace string
	pod       string
	crash     crashLog
}

// openPrevious opens the previous instance's log and starts reading it.
// The stream is opened before the view gives up the current one, so an
// error leaves the view as it was.
func openPrevious(ctx context.Context, cancel context.CancelFunc, tok async.Token, spec previousSpec, opts k8s.LogOptions) tea.Cmd {
	return func() tea.Msg {
		stream, err := spec.src.StreamLogs(ctx, spec.namespace, spec.pod, opts)
		if err != nil {
			cancel()
			return previousOpenedMsg{tok: tok, err: err}
		}
		s := newSession(ctx, cancel, tok, spec.src, spec.namespace, spec.crash, 1)
		go s.pump(pumpDirect, nil, 0)
		s.attachOpen(attachReq{pod: spec.pod}, stream)
		return previousOpenedMsg{tok: tok, sess: s}
	}
}

func (m Model) onPreviousOpened(msg previousOpenedMsg) (views.View, tea.Cmd) {
	if !m.opening.busy() || !m.opening.tok.Accept(msg.tok) {
		msg.sess.stop()
		return m, nil
	}
	m.opening = opening{}
	if msg.err != nil {
		return m, previousFailed(m.namespace, m.pods[0], m.container, msg.err)
	}
	m.previous = true
	m.follow, _ = m.follow.WithActive(false)
	// The current streams stop: nothing of theirs is explained, retried or
	// joined while the previous instance shows.
	m.streams = tracker{attempts: m.streams.attempts, ends: m.streams.ends}
	m.explain = explainer{gen: m.explain.gen}
	m.sess.stop()
	m.sess = msg.sess
	m.autoFollow = true
	m.clearLogs()
	return m, waitOrDrainCmd(m.sess)
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
	return statusCmd(status.Error, "previous logs of pod "+pod+": "+errText(err))
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
