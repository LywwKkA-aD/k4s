package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// handleRestorePromptKey is the keymap for the startup restore popup —
// y / Y / Enter revives every persisted forward in the state file;
// n / q / Esc dismisses without starting them. (Ctrl+C never reaches this
// handler: onKey intercepts it as force-quit before prompt dispatch.)
//
// "Revive" is best-effort: each Start call goes through the usual
// port-availability pre-check, so entries whose local port is now
// occupied (or privileged) end up in Error status and the user can
// retry from the forwards view.
func (m Model) handleRestorePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		return m.reviveAll(), nil
	case tea.KeyEsc:
		return m.dismissRestore(), nil
	}
	switch msg.String() {
	case "y", "Y":
		return m.reviveAll(), nil
	case "n", "N", "q":
		return m.dismissRestore(), nil
	}
	return m, nil
}

// reviveAll kicks off Start for every forward in state that belongs to the
// current context. Each Start blocks briefly on its own goroutine; we don't
// wait. The Manager's change channel will refresh the forwards view automatically.
func (m Model) reviveAll() Model {
	if m.forwardMgr != nil {
		state := m.forwardMgr.State()
		mgr := m.forwardMgr
		ctxName := currentContextName(m.client)
		for _, fwd := range state.Forwards {
			if fwd.Context != "" && fwd.Context != ctxName {
				continue
			}
			id := fwd.ID
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				_ = mgr.Start(ctx, id)
			}()
		}
	}
	m.pendingRestoreCount = 0
	m.cmdMode = cmdBarOff
	return m
}

func (m Model) dismissRestore() Model {
	m.pendingRestoreCount = 0
	m.cmdMode = cmdBarOff
	return m
}

// forwardsForContext counts persisted forwards that belong to the named
// context. Forwards with an empty context are treated as belonging to every
// context for backward compatibility with state files written before the
// context field was enforced.
func forwardsForContext(forwards []forwards.Forward, ctx string) int {
	n := 0
	for _, f := range forwards {
		if f.Context == "" || f.Context == ctx {
			n++
		}
	}
	return n
}

// renderRestorePromptInner builds the body of the startup restore popup.
// Shown when k4s loaded persisted port-forwards and needs the user to
// decide whether to bring them back. y / Enter revives all, n / Esc
// dismisses. Stays minimal — one keystroke either way.
func (m Model) renderRestorePromptInner() string {
	plural := "forwards"
	if m.pendingRestoreCount == 1 {
		plural = "forward"
	}
	title := styles.PopupTitle.Render("restore port-forwards")
	question := fmt.Sprintf("Found %d saved %s from a previous session.", m.pendingRestoreCount, plural)
	hint := styles.Hint.Render("y / Enter revive all · n / Esc skip · :pf to manage later")
	return lipgloss.JoinVertical(lipgloss.Left, title, "", question, "", hint)
}
