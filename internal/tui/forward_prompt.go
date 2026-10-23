package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// errForwardsUnavailable is shown when the port-forward state could not be
// loaded, so nothing can be registered.
var errForwardsUnavailable = errors.New("state subsystem unavailable")

// msgForwardsStopping is what a refused forward request or revival says
// while the previous context's sessions are still winding down.
const msgForwardsStopping = "previous context's forwards are still stopping"

// forwardsStopping reports whether the forwards Manager still refuses to
// register or start a forward, so the root can say so before it asks the
// user for anything.
func (m Model) forwardsStopping() bool {
	return m.forwardMgr != nil && m.forwardMgr.Blocked() != nil
}

// onForwardRequest opens the port prompt. The prompt parses the pair and
// probes the local port itself, so a typo or a busy port keeps it open with
// the error shown; only a usable pair comes back as ForwardChosenMsg.
func (m Model) onForwardRequest(msg views.ForwardRequestMsg) (tea.Model, tea.Cmd) {
	if m.forwardMgr == nil {
		return m.setError("port-forward: " + errForwardsUnavailable.Error())
	}
	if m.forwardsStopping() {
		return m.setError(msgForwardsStopping)
	}
	m.status = m.status.Clear()
	m.overlay = overlay.NewForward(msg, checkLocalPort)
	return m, nil
}

// checkLocalPort probes the local port before anything is saved, so the user
// can pick another instead of finding a born-dead entry in the forwards list.
func checkLocalPort(port uint16) error {
	if err := k8s.CheckLocalPortAvailable(port); err != nil {
		return errors.New(forwards.FriendlyBindError(err, port))
	}
	return nil
}

// onForwardChosen registers the forward, starts it, and opens the forwards
// view so the user sees the entry land.
func (m Model) onForwardChosen(msg overlay.ForwardChosenMsg) (tea.Model, tea.Cmd) {
	start, err := startForward(m.forwardMgr, currentContextName(m.client), msg)
	if err != nil {
		return m.setError("port-forward: " + err.Error())
	}
	next := m.switchTo(viewForwards)
	return next, tea.Batch(next.relayoutCmd(), start)
}

// startForward saves the forward and returns the Cmd that starts it: Start
// may block briefly while it resolves the target pod, and the Manager
// reports progress through its change channel.
func startForward(mgr *forwards.Manager, contextName string, msg overlay.ForwardChosenMsg) (tea.Cmd, error) {
	if mgr == nil {
		return nil, errForwardsUnavailable
	}
	fwd := forwards.Forward{
		ID:         forwards.NewID(),
		Context:    contextName,
		Namespace:  msg.Request.Namespace,
		Kind:       msg.Request.Kind,
		Name:       msg.Request.Name,
		LocalPort:  msg.Local,
		RemotePort: msg.Remote,
	}
	if err := mgr.Register(fwd); err != nil {
		return nil, err
	}
	return forwardStartCmd(mgr.Start, fwd.ID), nil
}

// forwardStartTimeout bounds one Start or Revive.
const forwardStartTimeout = 10 * time.Second

// forwardStartCmd runs start for the forward id off the event loop. An
// error goes to the status row: some, such as a forward refused for
// another cluster, end the start before its row could show them.
func forwardStartCmd(start func(context.Context, string) error, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), forwardStartTimeout)
		defer cancel()
		if err := start(ctx, id); err != nil {
			return views.StatusMsg{Level: status.Error, Text: "port-forward: " + sanitize.Line(err.Error())}
		}
		return nil
	}
}
