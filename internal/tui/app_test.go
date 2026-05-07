package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TestViewFillsTerminal asserts the chrome (header + footer) sits at the
// terminal edges and the body fills everything in between, regardless of
// how much content the active view actually rendered.
func TestViewFillsTerminal(t *testing.T) {
	t.Parallel()

	cases := []struct{ w, h int }{
		{100, 50},
		{120, 30},
		{80, 24},
		{200, 80},
	}

	for _, c := range cases {
		m := New(nil)
		upd, _ := m.Update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
		m = upd.(Model)

		out := m.View()
		if h := lipgloss.Height(out); h != c.h {
			t.Errorf("size=%dx%d: rendered height = %d, want %d (bodyHeight=%d)",
				c.w, c.h, h, c.h, m.bodyHeight())
		}
	}
}

// TestBodyPadsToBodyHeight is the focused regression test: the body block
// itself must be exactly bodyHeight tall, even when the active view's
// View() returns just one line ("loading…", "no kubeconfig", short table).
// Previously lipgloss.Place was used here and silently failed to pad.
func TestBodyPadsToBodyHeight(t *testing.T) {
	t.Parallel()

	m := New(nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = upd.(Model)

	bodyHeight := m.bodyHeight()
	body := lipgloss.NewStyle().
		Width(m.width).
		Height(bodyHeight).
		MaxHeight(bodyHeight).
		Render(m.current.View())

	if h := lipgloss.Height(body); h != bodyHeight {
		t.Errorf("body height = %d, want %d", h, bodyHeight)
	}
}

// TestNavigationHistoryAndGoHome simulates the dashboard → namespaces →
// pods(ns=demo) flow and asserts that Esc walks back through it correctly.
func TestNavigationHistoryAndGoHome(t *testing.T) {
	t.Parallel()

	m := New(nil)
	if got := m.current.Title(); got != viewDashboard {
		t.Fatalf("initial view = %q, want %q", got, viewDashboard)
	}

	// :ns — switch to namespaces. Pushes (dashboard, "").
	m = m.switchTo(viewNamespaces)
	if got := m.current.Title(); got != viewNamespaces {
		t.Fatalf("view = %q, want %q", got, viewNamespaces)
	}
	if len(m.history) != 1 || m.history[0].view != viewDashboard {
		t.Fatalf("history = %+v, want [{dashboard}]", m.history)
	}

	// Enter on a namespace. Mirrors the NamespaceSelectedMsg branch:
	// push pre-selection (namespaces, ""), set ns, replace with pods.
	m.history = append(m.history, historyEntry{
		view:      m.current.Title(),
		namespace: m.namespace,
	})
	m.namespace = "demo"
	m = m.replaceView(viewPods)
	if m.current.Title() != viewPods || m.namespace != "demo" {
		t.Fatalf("after pods/demo: view=%q ns=%q", m.current.Title(), m.namespace)
	}
	if len(m.history) != 2 {
		t.Fatalf("history len = %d, want 2", len(m.history))
	}

	// First Esc: namespaces with ns="" (the pre-selection state).
	popped, ok := m.popHistory()
	if !ok {
		t.Fatal("popHistory returned false on non-empty stack")
	}
	if popped.current.Title() != viewNamespaces || popped.namespace != "" {
		t.Fatalf("first pop: view=%q ns=%q (want namespaces \"\")",
			popped.current.Title(), popped.namespace)
	}

	// Second Esc: dashboard.
	popped, ok = popped.popHistory()
	if !ok {
		t.Fatal("popHistory returned false on non-empty stack")
	}
	if popped.current.Title() != viewDashboard {
		t.Fatalf("second pop: view = %q, want %q", popped.current.Title(), viewDashboard)
	}

	// Third Esc on empty history: no-op.
	if _, ok := popped.popHistory(); ok {
		t.Fatal("popHistory returned true on empty stack")
	}
}

func TestGoHomeClearsHistory(t *testing.T) {
	t.Parallel()

	m := New(nil)
	m = m.switchTo(viewNamespaces)
	m = m.switchTo(viewPods)
	if len(m.history) != 2 {
		t.Fatalf("history len = %d, want 2", len(m.history))
	}

	m = m.goHome()
	if m.current.Title() != viewDashboard {
		t.Fatalf("view = %q, want %q", m.current.Title(), viewDashboard)
	}
	if len(m.history) != 0 {
		t.Fatalf("history len = %d, want 0", len(m.history))
	}
}

// TestRelayoutPreservesTerminalHeight is the regression test for the bug
// where every view switch shrunk m.height by the chrome height — the footer
// crept up the screen until it disappeared off the top.
func TestRelayoutPreservesTerminalHeight(t *testing.T) {
	t.Parallel()

	m := New(nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 50})
	m = upd.(Model)
	if m.height != 50 {
		t.Fatalf("initial height = %d, want 50", m.height)
	}

	// Five back-to-back relayouts (one per view switch in the bug repro).
	for i := 0; i < 5; i++ {
		upd, _ = m.Update(relayoutMsg{})
		m = upd.(Model)
		if m.height != 50 {
			t.Fatalf("after %d relayouts, height = %d, want 50", i+1, m.height)
		}
		out := m.View()
		if h := lipgloss.Height(out); h != 50 {
			t.Fatalf("after %d relayouts, rendered height = %d, want 50", i+1, h)
		}
	}
}

func TestSwitchToSamePageDoesNotPushHistory(t *testing.T) {
	t.Parallel()

	m := New(nil)
	// dashboard -> dashboard should be a no-op for history.
	m = m.switchTo(viewDashboard)
	if len(m.history) != 0 {
		t.Errorf("switchTo on same view pushed history: %+v", m.history)
	}
}

// TestHelpPopupOpensAndCloses asserts '?' opens the help mode and the next
// key — any key — closes it again. The popup blocks key forwarding to the
// active view, so the dashboard underneath stays untouched.
func TestHelpPopupOpensAndCloses(t *testing.T) {
	t.Parallel()

	m := New(nil)
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = upd.(Model)

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = upd.(Model)
	if m.cmdMode != cmdBarHelp {
		t.Fatalf("cmdMode after '?' = %v, want cmdBarHelp", m.cmdMode)
	}

	out := m.View()
	if !contains(out, "help") {
		t.Errorf("help popup should render the title 'help'; got:\n%s", out)
	}

	upd, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = upd.(Model)
	if m.cmdMode != cmdBarOff {
		t.Fatalf("cmdMode after Esc = %v, want cmdBarOff", m.cmdMode)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
