package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// errForwardsUnavailable is shown when the port-forward state could not be
// loaded, so nothing can be registered.
var errForwardsUnavailable = errors.New("state subsystem unavailable")

// onForwardRequest opens the port prompt. The prompt parses the pair and
// probes the local port itself, so a typo or a busy port keeps it open with
// the error shown; only a usable pair comes back as ForwardChosenMsg.
func (m Model) onForwardRequest(msg views.ForwardRequestMsg) (tea.Model, tea.Cmd) {
	if m.forwardMgr == nil {
		return m.setError("port-forward: " + errForwardsUnavailable.Error())
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
	if err := startForward(m.forwardMgr, currentContextName(m.client), msg); err != nil {
		return m.setError("port-forward: " + err.Error())
	}
	next := m.switchTo(viewForwards)
	return next, next.relayoutCmd()
}

// startForward saves the forward and starts it in the background: Start may
// block briefly while it resolves the target pod, and the Manager reports
// progress through its change channel.
func startForward(mgr *forwards.Manager, contextName string, msg overlay.ForwardChosenMsg) error {
	if mgr == nil {
		return errForwardsUnavailable
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
		return err
	}
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.Start(ctx, id)
	}(fwd.ID)
	return nil
}
