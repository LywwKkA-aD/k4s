package overlay

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// BindCheck reports why a local port cannot be bound, nil when it can.
type BindCheck func(port uint16) error

// forward asks for the local:remote pair of a port-forward.
type forward struct {
	req   views.ForwardRequestMsg
	input textinput.Model
	check BindCheck
	err   string
}

// NewForward opens the port prompt for req, prefilled with a non-privileged
// local port for the remote one when req has a hint. Enter parses the pair
// and asks check about the local port: on an error the prompt stays open
// and shows it, otherwise it closes with ForwardChosenMsg. A nil check
// accepts every port.
func NewForward(req views.ForwardRequestMsg, check BindCheck) Overlay {
	placeholder, prefill := "8080:80", ""
	if req.RemotePort > 0 {
		placeholder = fmt.Sprintf("%d:%d", defaultLocalPort(req.RemotePort), req.RemotePort)
		prefill = placeholder
	}
	return forward{req: req, input: newInput("❯ ", placeholder, prefill), check: check}
}

// Update edits the pair; an edit clears the last error.
func (f forward) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case keys.IsEsc(k):
			return nil, nil
		case keys.IsEnter(k):
			return f.submit()
		}
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if f.input.Value() != before {
		f.err = ""
	}
	return f, cmd
}

func (f forward) submit() (Overlay, tea.Cmd) {
	local, remote, err := parsePortPair(strings.TrimSpace(f.input.Value()), f.req.RemotePort)
	if err == nil && f.check != nil {
		err = f.check(local)
	}
	if err != nil {
		f.err = sanitize.Line(err.Error())
		return f, nil
	}
	return nil, send(ForwardChosenMsg{Request: f.req, Local: local, Remote: remote})
}

// View shows the target, the input, the last error and the format.
func (f forward) View(width int) string {
	target := sanitize.Line(f.req.Kind) + "/" + sanitize.Line(f.req.Name)
	if f.req.Namespace != "" {
		target += " · " + sanitize.Line(f.req.Namespace)
	}
	lines := []string{
		styles.PopupTitle.Render("port-forward"),
		styles.Hint.Render(target),
		"",
		inputView(f.input, width),
		"",
	}
	if f.err != "" {
		lines = append(lines, styles.Err.Render(f.err))
	}
	lines = append(lines, styles.Hint.Render("Enter start · Esc cancel · format local:remote (e.g. 8080:80)"))
	return rows(width, lines...)
}

// defaultLocalPort moves a privileged remote port up by 8000 (80 → 8080,
// 443 → 8443) so a non-root user can bind it; others pass through.
func defaultLocalPort(remote uint16) uint16 {
	if remote < 1024 {
		return remote + 8000
	}
	return remote
}

// parsePortPair accepts "local:remote", "local" (remote from the hint, or
// the same port), ":remote" or "local:" (the same port on both sides), and
// "" (the hint on both sides).
func parsePortPair(raw string, remoteHint uint16) (local, remote uint16, err error) {
	if raw == "" {
		if remoteHint == 0 {
			return 0, 0, errors.New("port required (try 8080:80)")
		}
		return remoteHint, remoteHint, nil
	}
	localText, remoteText, paired := strings.Cut(raw, ":")
	if !paired {
		l, err := parsePort(localText)
		if err != nil {
			return 0, 0, err
		}
		if remoteHint == 0 {
			return l, l, nil
		}
		return l, remoteHint, nil
	}
	l, err := parseSide("local", localText)
	if err != nil {
		return 0, 0, err
	}
	r, err := parseSide("remote", remoteText)
	if err != nil {
		return 0, 0, err
	}
	switch {
	case l == 0 && r == 0:
		return 0, 0, errors.New("both ports empty")
	case l == 0:
		return r, r, nil
	case r == 0:
		return l, l, nil
	}
	return l, r, nil
}

// parseSide parses one side of a pair; empty is 0.
func parseSide(side, s string) (uint16, error) {
	if s == "" {
		return 0, nil
	}
	p, err := parsePort(s)
	if err != nil {
		return 0, fmt.Errorf("%s port: %w", side, err)
	}
	return p, nil
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if n == 0 {
		return 0, errors.New("port must be > 0")
	}
	return uint16(n), nil
}

// TakesText reports the port prompt's input: y is text there.
func (forward) TakesText() bool { return true }
