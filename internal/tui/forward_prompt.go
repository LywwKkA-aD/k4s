package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onForwardRequest opens the port prompt prefilled with "remote:remote"
// when we know the remote port, or "8080:" when we don't — so the user
// usually only has to press Enter. The actual port-forward fires from
// the submission path via dispatchForwardCmd.
func (m Model) onForwardRequest(msg views.ForwardRequestMsg) (tea.Model, tea.Cmd) {
	if m.forwardMgr == nil {
		return m.setCmdError("port-forward: state subsystem unavailable")
	}
	// Wipe any stale "port already in use" / "permission denied" line
	// from a previous attempt — re-opening the prompt should feel
	// clean, not haunted.
	m.cmdError = ""
	m.cmdMode = cmdBarForwardPrompt
	m.pendingForwardKind = msg.Kind
	m.pendingForwardNamespace = msg.Namespace
	m.pendingForwardName = msg.Name
	m.pendingForwardRemotePort = msg.RemotePort
	// The popup carries the target name in its subtitle, so the input
	// prompt itself stays terse — just the arrow.
	m.cmdBar.Prompt = "❯ "
	placeholder := "8080:80"
	prefill := ""
	if msg.RemotePort > 0 {
		local := defaultLocalPort(msg.RemotePort)
		placeholder = fmt.Sprintf("%d:%d", local, msg.RemotePort)
		prefill = placeholder
	}
	m.cmdBar.Placeholder = placeholder
	m.cmdBar.SetValue(prefill)
	m.cmdBar.CursorEnd()
	m.cmdBar.Focus()
	return m, textinput.Blink
}

// defaultLocalPort picks a non-privileged local port for a given remote.
// The rule is deliberately simple: anything < 1024 gets bumped by 8000
// (80 → 8080, 443 → 8443, 22 → 8022) so non-root users can bind it
// without "permission denied". Anything already non-privileged passes
// through unchanged.
func defaultLocalPort(remote uint16) uint16 {
	if remote < 1024 {
		return remote + 8000
	}
	return remote
}

// dispatchForward parses "local:remote" (or "local" with a known remote)
// and registers the forward with the Manager, then kicks off Start in a
// background goroutine — the Manager updates the UI via its change
// channel. We do **not** make this a tea.Cmd because Start may block
// briefly while resolving the target pod, which is fine off the loop.
func dispatchForward(m Model, mgr *forwards.Manager, kind, ns, name string, remoteHint uint16, raw string) (Model, error) {
	if mgr == nil {
		return m, fmt.Errorf("subsystem unavailable")
	}
	local, remote, err := parsePortPair(strings.TrimSpace(raw), remoteHint)
	if err != nil {
		return m, err
	}
	// Probe the local port before we persist anything to state. If
	// it's privileged or busy, surfacing the error here lets the user
	// retype with another port and avoids cluttering the forwards
	// list with born-dead entries.
	if err := k8s.CheckLocalPortAvailable(local); err != nil {
		return m, errors.New(forwards.FriendlyBindError(err, local))
	}
	fwd := forwards.Forward{
		ID:         forwards.NewID(),
		Context:    currentContextName(m.client),
		Namespace:  ns,
		Kind:       kind,
		Name:       name,
		LocalPort:  local,
		RemotePort: remote,
	}
	if err := mgr.Register(fwd); err != nil {
		return m, err
	}
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.Start(ctx, id)
	}(fwd.ID)
	return m, nil
}

// parsePortPair accepts "local:remote", "local" (uses remoteHint), or
// ":remote" (local := remote). Returns a clear error for anything else
// so the user sees a useful message instead of a port-forward that
// silently fails.
func parsePortPair(raw string, remoteHint uint16) (local, remote uint16, err error) {
	if raw == "" {
		if remoteHint == 0 {
			return 0, 0, fmt.Errorf("port required (try 8080:80)")
		}
		return remoteHint, remoteHint, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	switch len(parts) {
	case 1:
		l, perr := parsePort(parts[0])
		if perr != nil {
			return 0, 0, perr
		}
		if remoteHint == 0 {
			return l, l, nil
		}
		return l, remoteHint, nil
	case 2:
		var l, r uint16
		var perr error
		if parts[0] != "" {
			l, perr = parsePort(parts[0])
			if perr != nil {
				return 0, 0, fmt.Errorf("local port: %w", perr)
			}
		}
		if parts[1] != "" {
			r, perr = parsePort(parts[1])
			if perr != nil {
				return 0, 0, fmt.Errorf("remote port: %w", perr)
			}
		}
		switch {
		case l == 0 && r == 0:
			return 0, 0, fmt.Errorf("both ports empty")
		case l == 0:
			return r, r, nil
		case r == 0:
			return l, l, nil
		}
		return l, r, nil
	}
	return 0, 0, fmt.Errorf("invalid format: %q", raw)
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if n == 0 {
		return 0, fmt.Errorf("port must be > 0")
	}
	return uint16(n), nil
}

// renderForwardPromptInner builds the body of the port-forward popup:
// title + target + textinput for "local:remote" + hint with examples and
// the friendly-default heuristic (so the user knows where 8080 came
// from). The cmdBar's own prompt text gets cleared here so the popup
// can show a tidier label.
func (m Model) renderForwardPromptInner() string {
	target := fmt.Sprintf("%s/%s", m.pendingForwardKind, m.pendingForwardName)
	if m.pendingForwardNamespace != "" {
		target = fmt.Sprintf("%s · %s", target, m.pendingForwardNamespace)
	}
	title := styles.PopupTitle.Render("port-forward")
	subtitle := styles.Hint.Render(target)
	hint := styles.Hint.Render("Enter start · Esc cancel · format local:remote (e.g. 8080:80)")
	return lipgloss.JoinVertical(lipgloss.Left, title, subtitle, "", m.cmdBar.View(), "", hint)
}
