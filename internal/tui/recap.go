package tui

import (
	"fmt"
	"io"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// maxRecap is how many commands the exit recap prints.
const maxRecap = 15

// recapTitle heads the recap in the shell's scrollback.
const recapTitle = "kubectl commands from this k4s session:"

// recap is the distinct kubectl commands the session led to, as the
// footer showed them (decorated, ≈ included), least recently used first.
// A command seen again moves to the end; past maxRecap the least recently
// used goes. It is a value: add returns a new recap.
type recap struct {
	lines []string
}

func (r recap) add(c kubectl.Cmd) recap {
	line := sanitize.Line(c.Display())
	if line == "" {
		return r
	}
	out := make([]string, 0, len(r.lines)+1)
	for _, l := range r.lines {
		if l != line {
			out = append(out, l)
		}
	}
	out = append(out, line)
	return recap{lines: out[max(len(out)-maxRecap, 0):]}
}

// noteRecap records the command behind an exec or a port-forward the user
// asked for. View opens, describe and logs are recorded when their view is
// installed (swap). Any other message changes nothing.
func (m Model) noteRecap(msg tea.Msg) Model {
	var cmd kubectl.Cmd
	switch msg := msg.(type) {
	case views.ExecRequestMsg:
		cmd, _ = views.ExecCommand(msg)
	case overlay.ForwardChosenMsg:
		cmd = portForwardCommand(msg)
	default:
		return m
	}
	m.recap = m.recap.add(m.decorate(cmd))
	return m
}

// portForwardCommand is `kubectl port-forward -n <ns> <kind>/<name> <l>:<r>`
// for the forward the user chose, the zero Cmd for names kubectl would
// reject. It matches the forwards view's command for the same row.
func portForwardCommand(c overlay.ForwardChosenMsg) kubectl.Cmd {
	r := c.Request
	if kubectl.CheckLabel("namespace", r.Namespace) != nil || kubectl.CheckSubdomain(r.Kind, r.Name) != nil {
		return kubectl.Cmd{}
	}
	return kubectl.New("port-forward", "-n", r.Namespace, r.Kind+"/"+r.Name, fmt.Sprintf("%d:%d", c.Local, c.Remote))
}

// Recap returns the kubectl commands the session led to, oldest first, at
// most 15 and each one once: the views opened, describe, logs, exec and
// port-forward, with the flags a pasted copy needs.
func (m Model) Recap() []string {
	return slices.Clone(m.recap.lines)
}

// WriteRecap prints lines (see Recap) under a title, one command per line,
// so they stay in the shell's scrollback after the TUI has gone. Nothing
// is printed for no lines.
func WriteRecap(w io.Writer, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, recapTitle); err != nil {
		return fmt.Errorf("write recap: %w", err)
	}
	for _, l := range lines {
		if _, err := fmt.Fprintln(w, "  "+l); err != nil {
			return fmt.Errorf("write recap: %w", err)
		}
	}
	return nil
}
