package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
)

// TestMain isolates every test in this package from the user's real
// $HOME by pointing XDG_STATE_HOME at a throwaway directory. Without
// this, New() loads ~/.local/state/k4s/portforwards.json from the
// developer's machine and may open the restore popup unbidden — which
// would break tests that assume a clean cmdBarOff start.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "k4s-tui-test-*")
	if err != nil {
		panic(err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state")); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

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

func TestBuildExecCommandIncludesContext(t *testing.T) {
	t.Parallel()
	cmd := buildExecCommand("staging", "ns", "pod-1", "app")
	args := cmd.Args
	want := []string{"kubectl", "exec", "-it", "--context", "staging", "-n", "ns", "-c", "app", "pod-1", "--", "sh"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

func TestBuildExecCommandOmitsEmptyContext(t *testing.T) {
	t.Parallel()
	cmd := buildExecCommand("", "ns", "pod-1", "")
	args := cmd.Args
	if containsArg(args, "--context") {
		t.Errorf("empty context should not add --context flag: %v", args)
	}
	if containsArg(args, "-c") {
		t.Errorf("empty container should not add -c flag: %v", args)
	}
}

func containsArg(args []string, target string) bool {
	for _, a := range args {
		if a == target {
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
	if isRebuildableView("logs · pod-x") {
		t.Error("dynamic log title should not be rebuildable")
	}
	if isRebuildableView("pod · nginx") {
		t.Error("dynamic describe title should not be rebuildable")
	}
}

func TestForwardsForContext(t *testing.T) {
	t.Parallel()
	fwd := func(ctx string) forwards.Forward { return forwards.Forward{Context: ctx} }
	forwards := []forwards.Forward{fwd("prod"), fwd("dev"), fwd(""), fwd("prod")}
	if got := forwardsForContext(forwards, "prod"); got != 3 {
		t.Errorf("prod count = %d, want 3", got)
	}
	if got := forwardsForContext(forwards, "dev"); got != 2 {
		t.Errorf("dev count = %d, want 2", got)
	}
}

func TestCmdErrorAutoClearsAfterTTL(t *testing.T) {
	t.Parallel()

	m := New(nil)
	m, cmd := m.setCmdError("exec: boom")
	if m.cmdError == "" {
		t.Fatal("setCmdError did not record the error")
	}
	if cmd == nil {
		t.Fatal("setCmdError did not arm the auto-clear timer")
	}

	// A stale timer must not wipe a newer error.
	m.cmdError = "newer error"
	upd, _ := m.Update(cmdErrorClearMsg{value: "exec: boom"})
	m = upd.(Model)
	if m.cmdError != "newer error" {
		t.Fatalf("stale timer cleared newer error: cmdError = %q", m.cmdError)
	}

	// The matching timer clears it.
	upd, _ = m.Update(cmdErrorClearMsg{value: "newer error"})
	m = upd.(Model)
	if m.cmdError != "" {
		t.Fatalf("matching timer did not clear: cmdError = %q", m.cmdError)
	}
}
