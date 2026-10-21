package forwards

// These tests interleave manager changes with the view's own steps, which
// only the package itself can drive one at a time. Like the external tests,
// they own a state directory through t.Setenv and do not run in parallel.

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
)

func testManager(t *testing.T) *forwards.Manager {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, err := forwards.NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func register(t *testing.T, mgr *forwards.Manager, name string) {
	t.Helper()
	f := forwards.Forward{ID: forwards.NewID(), Context: "fake", Namespace: "demo", Kind: "pod", Name: name, LocalPort: 18080, RemotePort: 80}
	if err := mgr.Register(f); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

// mustReturn runs cmd and fails unless it returns within a second: a wait
// that missed a change would block until the next one.
func mustReturn(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("no Cmd to run")
	}
	got := make(chan tea.Msg, 1)
	go func() { got <- cmd() }()
	select {
	case msg := <-got:
		return msg
	case <-time.After(time.Second):
		t.Fatal("the change watcher missed a change")
		return nil
	}
}

func shows(m Model, name string) bool {
	for _, a := range m.shown {
		if a.Forward.Name == name {
			return true
		}
	}
	return false
}

// A change that lands after the view listed, but before its wait starts,
// is still seen: the view takes Changes before List, in New (for Init) and
// on every change.
func TestChangeBetweenListAndWaitIsSeen(t *testing.T) {
	mgr := testManager(t)
	m := New(mgr)
	t.Cleanup(func() { _ = m.Close() })

	register(t, mgr, "alpha")
	next, cmd := m.Update(mustReturn(t, m.Init()))
	m = next.(Model)
	if !shows(m, "alpha") {
		t.Fatal("the change before Init's wait was not shown")
	}

	register(t, mgr, "bravo")
	next, _ = m.Update(mustReturn(t, cmd))
	if !shows(next.(Model), "bravo") {
		t.Error("the change before the next wait was not shown")
	}
}

// A watcher of a closed view may have fired before Close; its message must
// not arm a second watcher in the view that replaced it.
func TestAnotherViewsChangeIsDropped(t *testing.T) {
	mgr := testManager(t)
	old := New(mgr)
	wait := old.Init()
	register(t, mgr, "alpha")
	stale := mustReturn(t, wait)
	_ = old.Close()

	m := New(mgr)
	t.Cleanup(func() { _ = m.Close() })
	if _, cmd := m.Update(stale); cmd != nil {
		t.Error("a change seen by another view armed a watcher here")
	}
}
