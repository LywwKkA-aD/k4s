package tui

import "testing"

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

func TestSwitchToSamePageDoesNotPushHistory(t *testing.T) {
	t.Parallel()

	m := New(nil)
	// dashboard -> dashboard should be a no-op for history.
	m = m.switchTo(viewDashboard)
	if len(m.history) != 0 {
		t.Errorf("switchTo on same view pushed history: %+v", m.history)
	}
}
