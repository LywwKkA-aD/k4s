package forwards

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// startTimeout caps how long a start may resolve the target and wait for
// the tunnel; the Manager marks the forward failed when it runs out.
const startTimeout = 10 * time.Second

// The operations a row key runs, as the status line names them.
const (
	opStart   = "start"
	opStop    = "stop"
	opRestart = "restart"
	opRemove  = "remove"
)

// forwardOpResultMsg reports how an operation on fwd ended. The view
// re-reads its rows and puts the outcome on the status line.
type forwardOpResultMsg struct {
	op  string
	fwd forwards.Forward
	err error
	// boundTo names the cluster a forward saved before k4s recorded
	// servers was saved for by this start.
	boundTo string
}

// startCmd and the other operations block (Start resolves the target pod,
// Stop waits for the session to wind down), so they run as Cmds.
func startCmd(mgr *forwards.Manager, a forwards.Active) tea.Cmd {
	return func() tea.Msg {
		return started(mgr, a, opStart, start(mgr, a.Forward))
	}
}

// started reports a start of a. Starting a forward whose row asked to
// confirm its cluster is the confirmation, so the status says where it is
// saved now.
func started(mgr *forwards.Manager, a forwards.Active, op string, err error) forwardOpResultMsg {
	msg := forwardOpResultMsg{op: op, fwd: a.Forward, err: err}
	if a.Unbound && err == nil {
		msg.boundTo = mgr.Where()
	}
	return msg
}

func stopCmd(mgr *forwards.Manager, f forwards.Forward) tea.Cmd {
	return func() tea.Msg {
		return forwardOpResultMsg{op: opStop, fwd: f, err: mgr.Stop(f.ID)}
	}
}

// restartCmd starts the forward again only once the old session is down:
// one still closing may hold the local port. A Manager that refuses starts
// is asked first, so the refusal does not leave the forward stopped.
func restartCmd(mgr *forwards.Manager, a forwards.Active) tea.Cmd {
	return func() tea.Msg {
		err := mgr.Blocked()
		if err == nil {
			err = mgr.Stop(a.Forward.ID)
		}
		if err == nil {
			err = start(mgr, a.Forward)
		}
		return started(mgr, a, opRestart, err)
	}
}

func removeCmd(mgr *forwards.Manager, f forwards.Forward) tea.Cmd {
	return func() tea.Msg {
		return forwardOpResultMsg{op: opRemove, fwd: f, err: mgr.Remove(f.ID)}
	}
}

// start revives f: the user picked its row, which is the confirmation a
// forward saved for an unknown cluster waits for.
func start(mgr *forwards.Manager, f forwards.Forward) error {
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	return mgr.Revive(ctx, f.ID)
}

// status is the line the outcome puts in the footer.
func (r forwardOpResultMsg) status() views.StatusMsg {
	name := target(r.fwd)
	switch {
	case errors.Is(r.err, forwards.ErrNotFound):
		return views.StatusMsg{Level: status.Warn, Text: fmt.Sprintf("%s %s: the forward is gone", r.op, name)}
	case r.err != nil:
		return views.StatusMsg{Level: status.Error, Text: fmt.Sprintf("%s %s: %v", r.op, name, r.err)}
	}
	text := fmt.Sprintf("forwarding localhost:%d → %s:%d", r.fwd.LocalPort, name, r.fwd.RemotePort)
	if r.boundTo != "" {
		text += " (saved for " + r.boundTo + ")"
	}
	switch r.op {
	case opStop:
		text = fmt.Sprintf("stopped %s (%s)", name, ports(r.fwd))
	case opRemove:
		text = fmt.Sprintf("removed %s (%s)", name, ports(r.fwd))
	}
	return views.StatusMsg{Level: status.OK, Text: text}
}

func statusCmd(msg views.StatusMsg) tea.Cmd {
	return func() tea.Msg { return msg }
}
