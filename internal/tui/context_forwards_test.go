package tui

// These tests put a Manager with hung sessions into the model, so they live
// inside the package. They give the port-forward state its own directory
// with t.Setenv and do not run in parallel.

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hungTunneler opens sessions that are ready at once and then ignore
// StopCh, like forwarders whose connections hang on close. They end when
// the test does.
type hungTunneler struct {
	mu   sync.Mutex
	done []chan struct{}
}

func (h *hungTunneler) StartPodPortForward(_ context.Context, _, pod string, _, _ uint16) (*k8s.PortForwardSession, error) {
	s := &k8s.PortForwardSession{
		StopCh: make(chan struct{}),
		Ready:  make(chan struct{}),
		Err:    make(chan error, 1),
		Done:   make(chan struct{}),
		Pod:    pod,
		Stderr: func() string { return "" },
		Stdout: func() string { return "" },
	}
	close(s.Ready)
	h.mu.Lock()
	h.done = append(h.done, s.Done)
	h.mu.Unlock()
	return s, nil
}

func (h *hungTunneler) ResolveServiceToPod(_ context.Context, _, name string, port int32) (*corev1.Pod, int32, error) {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name}}, port, nil
}

func (h *hungTunneler) ResolveDeploymentToPod(_ context.Context, _, name string) (string, error) {
	return name, nil
}

func (h *hungTunneler) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, d := range h.done {
		close(d)
	}
	h.done = nil
}

// openPort returns a local port nothing listens on right now: the Manager
// probes the real port before it starts a forward.
func openPort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return uint16(port) //nolint:gosec // a TCP port always fits
}

// hungForwards returns a Manager running n forwards whose sessions hang
// until the tunneler releases them.
func hungForwards(t *testing.T, n int) (*forwards.Manager, *hungTunneler) {
	t.Helper()
	tun := &hungTunneler{}
	t.Cleanup(tun.release)
	mgr, err := forwards.NewManager(nil, forwards.WithTunneler(tun))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for i := range n {
		f := forwards.Forward{
			ID: forwards.NewID(), Context: "elsewhere", Namespace: tuitest.NamespaceDemo,
			Kind: "pod", Name: fmt.Sprintf("hung-%d", i), LocalPort: openPort(t), RemotePort: 80,
		}
		if err := mgr.Register(f); err != nil {
			t.Fatalf("Register: %v", err)
		}
		if err := mgr.Start(t.Context(), f.ID); err != nil {
			t.Fatalf("Start: %v", err)
		}
	}
	return mgr, tun
}

// switchContext delivers a context switch to a model whose forwards are
// mgr's, and reports how long Update took.
func switchContext(t *testing.T, mgr *forwards.Manager) (Model, tea.Cmd, time.Duration) {
	t.Helper()
	m := New(tuitest.NewFakeClient(t), nil)
	m.forwardMgr = mgr
	begin := time.Now()
	next, cmd := m.Update(views.ContextSelectedMsg{Name: testContextBeta})
	return next.(Model), cmd, time.Since(begin)
}

// forwardsStatus runs every Cmd in the tree on its own goroutine and
// returns the status the forwards stop reports. Cmds that outlive the wait,
// such as the status line's expiry tick, are left behind.
func forwardsStatus(t *testing.T, cmd tea.Cmd, wait time.Duration) (views.StatusMsg, bool) {
	t.Helper()
	found := make(chan views.StatusMsg, 8)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		if c == nil {
			return
		}
		go func() {
			switch msg := c().(type) {
			case tea.BatchMsg:
				for _, child := range msg {
					run(child)
				}
			case views.StatusMsg:
				if strings.HasPrefix(msg.Text, "forwards: ") || strings.HasPrefix(msg.Text, "stopped ") {
					found <- msg
				}
			}
		}()
	}
	run(cmd)
	select {
	case msg := <-found:
		return msg, true
	case <-time.After(wait):
		return views.StatusMsg{}, false
	}
}

func requireStatusLine(t *testing.T, m Model, level status.Level, text string) {
	t.Helper()
	got, ok := m.status.Current(m.now())
	if !ok || got.Level != level || got.Text != text {
		t.Errorf("status line = %v %q (shown %v), want %v %q", got.Level, got.Text, ok, level, text)
	}
}

func TestContextSwitchStopsHungForwardsInTheBackground(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 3)
	m, cmd, took := switchContext(t, mgr)
	begin := time.Now().Add(-took)

	t.Logf("Update took %v", took)
	if took >= 10*time.Millisecond {
		t.Errorf("Update took %v with 3 hung forwards, want under 10ms", took)
	}
	requireStatusLine(t, m, status.Info, "stopping 3 forwards…")
	if m.forwardMgr == nil {
		t.Fatal("no Manager for the new context")
	}

	// A forward in the new context waits for the old sessions: its local
	// port may still be theirs.
	chosen := overlay.ForwardChosenMsg{
		Request: views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1},
		Local:   openPort(t), Remote: 80,
	}
	next, _ := m.Update(chosen)
	requireStatusLine(t, next.(Model), status.Error, "port-forward: "+forwards.ErrStopping.Error())
	if n := len(m.forwardMgr.State().Forwards); n != 3 {
		t.Errorf("a refused forward was saved: %d forwards in state, want the 3 hung ones", n)
	}

	msg, ok := forwardsStatus(t, cmd, 3*time.Second)
	if !ok {
		t.Fatal("the stop never reported")
	}
	if done := time.Since(begin); done >= 2500*time.Millisecond {
		t.Errorf("the stop reported after %v, want under 2.5s", done)
	}
	want := views.StatusMsg{Level: status.Warn, Text: "forwards: stop timed out: 3 of 3 forwards still closing"}
	if msg != want {
		t.Errorf("stop reported %+v, want %+v", msg, want)
	}
	if err := m.forwardMgr.Blocked(); err != nil {
		t.Errorf("the new Manager is still blocked after the stop ended: %v", err)
	}
	next, _ = next.Update(msg)
	requireStatusLine(t, next.(Model), status.Warn, want.Text)
}

func TestContextSwitchReportsAStopThatEnded(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, tun := hungForwards(t, 1)
	m, cmd, _ := switchContext(t, mgr)
	requireStatusLine(t, m, status.Info, "stopping 1 forward…")
	for _, a := range mgr.List() {
		if a.Status != forwards.StatusStopped {
			t.Errorf("old forward %s is %v after the switch, want stopped", a.Forward.Name, a.Status)
		}
	}

	// The session winds down after all.
	tun.release()
	msg, ok := forwardsStatus(t, cmd, 3*time.Second)
	want := views.StatusMsg{Level: status.OK, Text: "stopped 1 forward"}
	if !ok || msg != want {
		t.Errorf("stop reported %+v (%v), want %+v", msg, ok, want)
	}
}

func TestContextSwitchWithNothingRunning(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	mgr, _ := hungForwards(t, 0)
	m, cmd, _ := switchContext(t, mgr)
	if got, ok := m.status.Current(m.now()); ok {
		t.Errorf("status line = %q, want none when nothing was running", got.Text)
	}
	if err := m.forwardMgr.Blocked(); err != nil {
		t.Errorf("Blocked = %v, want nil", err)
	}
	if _, ok := forwardsStatus(t, cmd, 300*time.Millisecond); ok {
		t.Error("a stop with nothing to stop reported")
	}
}
