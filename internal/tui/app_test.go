package tui

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
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
		m := New(nil, nil)
		upd, _ := m.Update(tuitest.Resize(c.w, c.h))
		m = upd.(Model)

		out := m.View().Content
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

	m := New(nil, nil)
	upd, _ := m.Update(tuitest.Resize(100, 50))
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

	m := New(nil, nil)
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
	popped, _, ok := m.popHistory()
	if !ok {
		t.Fatal("popHistory returned false on non-empty stack")
	}
	if popped.current.Title() != viewNamespaces || popped.namespace != "" {
		t.Fatalf("first pop: view=%q ns=%q (want namespaces \"\")",
			popped.current.Title(), popped.namespace)
	}

	// Second Esc: dashboard.
	popped, _, ok = popped.popHistory()
	if !ok {
		t.Fatal("popHistory returned false on non-empty stack")
	}
	if popped.current.Title() != viewDashboard {
		t.Fatalf("second pop: view = %q, want %q", popped.current.Title(), viewDashboard)
	}

	// Third Esc on empty history: no-op.
	if _, _, ok := popped.popHistory(); ok {
		t.Fatal("popHistory returned true on empty stack")
	}
}

func TestGoHomeClearsHistory(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
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

	m := New(nil, nil)
	upd, _ := m.Update(tuitest.Resize(100, 50))
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
		out := m.View().Content
		if h := lipgloss.Height(out); h != 50 {
			t.Fatalf("after %d relayouts, rendered height = %d, want 50", i+1, h)
		}
	}
}

// TestStartupLoadErrorSurvivesNavigation asserts the dashboard keeps showing
// the kubeconfig load error after the user wanders off and comes back home.
func TestStartupLoadErrorSurvivesNavigation(t *testing.T) {
	t.Parallel()

	loadErr := errors.New("load kubeconfig: stat /nowhere/config: no such file or directory")
	m := New(nil, loadErr)
	upd, _ := m.Update(tuitest.Resize(160, 30))
	m = upd.(Model)

	if out := m.View().Content; !strings.Contains(out, "/nowhere/config") {
		t.Fatalf("landing view does not show the load error:\n%s", out)
	}
	m = m.switchTo(viewPods).goHome()
	if out := m.View().Content; !strings.Contains(out, "/nowhere/config") {
		t.Fatalf("dashboard lost the load error after navigation:\n%s", out)
	}
}

func TestSwitchToSamePageDoesNotPushHistory(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
	// dashboard -> dashboard should be a no-op for history.
	m = m.switchTo(viewDashboard)
	if len(m.history) != 0 {
		t.Errorf("switchTo on same view pushed history: %+v", m.history)
	}
}

// TestHelpPopupOpensAndCloses asserts '?' opens help as the one overlay and
// Esc closes it without going back: the dashboard underneath stays.
func TestHelpPopupOpensAndCloses(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
	upd, _ := m.Update(tuitest.Resize(100, 40))
	m = upd.(Model)

	upd, _ = m.Update(tuitest.Press("?"))
	m = upd.(Model)
	if m.overlay == nil || overlay.PlacementOf(m.overlay) != overlay.Popup {
		t.Fatalf("overlay after '?' = %T, want the help popup", m.overlay)
	}

	out := m.View().Content
	if !contains(out, "help") {
		t.Errorf("help popup should render the title 'help'; got:\n%s", out)
	}

	upd, _ = m.Update(tuitest.Esc())
	m = upd.(Model)
	if m.overlay != nil {
		t.Fatalf("overlay after Esc = %T, want none", m.overlay)
	}
	if m.current.Title() != viewDashboard {
		t.Fatalf("Esc on help left the dashboard for %q", m.current.Title())
	}
}

// TestModelKeepsOneOverlayField pins M3.1: prompts and popups live in the
// overlay field, not in per-prompt pending state.
func TestModelKeepsOneOverlayField(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[Model]()
	overlays := 0
	for i := range typ.NumField() {
		f := typ.Field(i)
		if strings.HasPrefix(f.Name, "pending") || f.Name == "cmdMode" || f.Name == "cmdBar" {
			t.Errorf("Model still has %s", f.Name)
		}
		if f.Type == reflect.TypeFor[overlay.Overlay]() {
			overlays++
		}
	}
	if overlays != 1 {
		t.Errorf("Model has %d overlay fields, want 1", overlays)
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

func TestIsRebuildableView(t *testing.T) {
	t.Parallel()
	for _, name := range []string{viewDashboard, viewPods, viewNamespaces, viewDeployments, viewServices, viewContexts, viewTop, viewForwards} {
		if !isRebuildableView(name) {
			t.Errorf("%q should be rebuildable", name)
		}
	}
	if isRebuildableView("pod · pod-x · logs") {
		t.Error("dynamic log title should not be rebuildable")
	}
	if isRebuildableView("pod · nginx") {
		t.Error("dynamic describe title should not be rebuildable")
	}
}

// TestStatusExpiryStaysWithTheRoot: the status row's expiry is the root's
// own message, never the active view's.
func TestStatusExpiryStaysWithTheRoot(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
	m, cmd := m.setError("exec: boom")
	if cmd == nil {
		t.Fatal("setError did not arm the expiry")
	}
	upd, cmd := m.Update(status.ExpireMsg{})
	if cmd != nil {
		t.Fatal("a stray expiry produced a Cmd")
	}
	if msg, ok := upd.(Model).status.Current(time.Now()); !ok || msg.Text != "exec: boom" {
		t.Fatalf("a stray expiry cleared the status: %+v, %v", msg, ok)
	}
}
