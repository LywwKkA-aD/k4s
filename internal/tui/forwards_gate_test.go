package tui

// The root refuses forward requests and revivals while the previous
// context's stop runs. Like context_forwards_test.go these tests use a
// private state directory and do not run in parallel.

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const stillStopping = "previous context's forwards are still stopping"

func TestSecondContextSwitchKeepsTheGateClosedUntilTheFirstStopEnds(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, tun := hungForwards(t, 3)

	m := New(tuitest.NewFakeClient(t), nil)
	m.forwardMgr = mgr
	next, firstCmd := m.Update(views.ContextSelectedMsg{Name: testContextBeta})
	begin := time.Now()
	next, secondCmd := next.(Model).Update(views.ContextSelectedMsg{Name: testContextAlpha})
	if took := time.Since(begin); took >= switchBudget {
		t.Errorf("the second Update took %v, want under %v", took, switchBudget)
	}
	m = next.(Model)

	if err := m.forwardMgr.Blocked(); err == nil {
		t.Fatal("the gate opened at once: the first stop is still running")
	}
	// The second switch has no sessions to stop, so it reports nothing; the
	// first stop's report is the one that lands.
	if msg, ok := forwardsStatus(t, secondCmd, 200*time.Millisecond); ok {
		t.Errorf("the second switch reported %+v", msg)
	}
	if err := m.forwardMgr.Blocked(); err == nil {
		t.Error("the gate opened before the first stop ended")
	}

	tun.release()
	msg, ok := forwardsStatus(t, firstCmd, 3*time.Second)
	if want := (views.StatusMsg{Level: status.OK, Text: "stopped 3 forwards"}); !ok || msg != want {
		t.Errorf("the first stop reported %+v (%v), want %+v", msg, ok, want)
	}
	deadline := time.Now().Add(5 * time.Second)
	for m.forwardMgr.Blocked() != nil {
		if time.Now().After(deadline) {
			t.Fatal("the gate never opened after the first stop ended")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestForwardRequestRefusedWhileThePreviousContextStops(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 1)
	m, _, _ := switchContext(t, mgr)

	next, _ := m.Update(views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	got := next.(Model)
	if got.overlay != nil {
		t.Error("the port prompt opened while the gate is closed")
	}
	requireStatusLine(t, got, status.Error, stillStopping)
}

func TestForwardRequestOpensThePromptOnceTheGateIsOpen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 0)
	m, _, _ := switchContext(t, mgr)

	next, _ := m.Update(views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	if next.(Model).overlay == nil {
		t.Error("the port prompt did not open with nothing stopping")
	}
}

func TestRevivalRefusedWhileThePreviousContextStops(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 1)
	m, _, _ := switchContext(t, mgr)

	next, _ := m.Update(overlay.RestoreDecisionMsg{Revive: true})
	requireStatusLine(t, next.(Model), status.Error, stillStopping)
}

func TestDecliningARevivalSaysNothing(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 1)
	m, _, _ := switchContext(t, mgr)

	next, _ := m.Update(overlay.RestoreDecisionMsg{Revive: false})
	got := next.(Model)
	if line, ok := got.status.Current(got.now()); ok && line.Text == stillStopping {
		t.Errorf("declining a revival said %q", line.Text)
	}
}
