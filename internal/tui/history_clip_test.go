package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestEscOnEarlierModelIgnoresLaterNavigation pins T-13 for the history
// stack: a pop must not leave spare capacity that a later push on the same
// copy writes into, or an earlier copy of the model would return to a view
// it never visited.
func TestEscOnEarlierModelIgnoresLaterNavigation(t *testing.T) {
	t.Parallel()

	// The command bar answers through a Cmd, so its messages are driven.
	send := func(m tea.Model, msgs ...tea.Msg) tea.Model {
		next, _ := tuitest.Drive(t, m.(Model), msgs...)
		return next
	}
	esc := tuitest.Esc()
	command := func(m tea.Model, name string) tea.Model {
		return send(m, tuitest.Press(":"), tuitest.Type(name), tuitest.Enter())
	}
	header := func(m tea.Model) string {
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
		return lines[0]
	}

	m := send(New(nil, nil), tea.WindowSizeMsg{Width: 120, Height: 30})
	m = command(m, "pods")
	m = command(m, "deploy")
	snapshot := m

	branch := send(snapshot, esc, esc)
	if got := header(branch); !strings.Contains(got, "view: dashboard") {
		t.Fatalf("two Esc presses should land on the dashboard, header: %q", got)
	}
	branch = command(branch, "svc")
	branch = command(branch, "top")
	if got := header(branch); !strings.Contains(got, "view: top") {
		t.Fatalf("branch should be on top, header: %q", got)
	}

	back := send(snapshot, esc)
	if got := header(back); !strings.Contains(got, "view: pods") {
		t.Errorf("Esc on the earlier model should return to pods, header: %q", got)
	}
}
