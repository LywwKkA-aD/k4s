package forwards

import (
	"context"
	"errors"
	"net"
	"testing"
)

// freePort returns a local port nothing listens on right now. NewManager
// probes the real port, so a fixed one could be taken on a developer's
// machine.
func freePort(t *testing.T) uint16 {
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

// TestNewManagerOptions cannot run in parallel: it points XDG_STATE_HOME at
// a temporary directory.
func TestNewManagerOptions(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	oldFake := newFakeTunneler(t)
	old, err := NewManager(nil, WithTunneler(oldFake))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	port := freePort(t)
	mustRegister(t, old, testForward("f1", "pod", port))
	sessions := running(t, old, oldFake, "f1")
	if call := oldFake.lastCall(t); call.pod != "web" || call.local != port {
		t.Errorf("WithTunneler: Start went to %+v", call)
	}

	stop := old.BeginStopAll()
	next, err := NewManager(nil, WithTunneler(newFakeTunneler(t)), After(stop))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := stop.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !sessions[0].isStopped() {
		t.Error("the old session is still open")
	}
	if err := next.Blocked(); err != nil {
		t.Errorf("Blocked after the stop = %v, want nil", err)
	}
	if err := next.Start(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Start(missing) = %v, want ErrNotFound", err)
	}
}
