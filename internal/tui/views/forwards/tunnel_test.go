package forwards

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// refusingTunneler opens sessions that come up at once and report that
// their pod refused a connection at failedAt.
type refusingTunneler struct {
	failedAt time.Time

	mu    sync.Mutex
	stops []chan struct{}
}

func (r *refusingTunneler) StartPodPortForward(_ context.Context, _, pod string, _, _ uint16) (*k8s.PortForwardSession, error) {
	s := &k8s.PortForwardSession{
		StopCh: make(chan struct{}),
		Ready:  make(chan struct{}),
		Err:    make(chan error, 1),
		Done:   make(chan struct{}),
		Pod:    pod,
		Stderr: func() string { return "" },
		LastError: func() k8s.TunnelError {
			return k8s.TunnelError{Message: "dial tcp4 127.0.0.1:80: connect: connection refused \x1b]0;pwned\x07", At: r.failedAt}
		},
	}
	close(s.Ready)
	go func() {
		<-s.StopCh
		close(s.Done)
	}()
	r.mu.Lock()
	r.stops = append(r.stops, s.StopCh)
	r.mu.Unlock()
	return s, nil
}

func (r *refusingTunneler) ResolveServiceToPod(_ context.Context, _, name string, port int32) (*corev1.Pod, int32, error) {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name}}, port, nil
}

func (r *refusingTunneler) ResolveDeploymentToPod(_ context.Context, _, name string) (string, error) {
	return name, nil
}

func freeLocalPort(t *testing.T) uint16 {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return uint16(port) //nolint:gosec // a TCP port always fits
}

func screen(m Model) string {
	return ansi.Strip(m.View())
}

func TestDetail(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	now := at.Add(3 * time.Second)
	lost := errors.New("lost connection to pod")
	cases := []struct {
		name string
		a    forwards.Active
		want string
	}{
		{name: "running", a: forwards.Active{Status: forwards.StatusRunning}, want: ""},
		{name: "failed", a: forwards.Active{Status: forwards.StatusError, Err: lost}, want: "lost connection to pod"},
		{
			name: "ended by a refused connection",
			a:    forwards.Active{Status: forwards.StatusError, Err: lost, TunnelErr: "connection refused", TunnelErrAt: at},
			want: "last error: connection refused · 3s ago",
		},
		{name: "unknown cluster", a: forwards.Active{Unbound: true}, want: "unknown cluster, confirm"},
		{name: "error text from the cluster", a: forwards.Active{Err: errors.New("pods \"x\" not found\x1b[2J")}, want: `pods "x" not found`},
	}
	for _, tc := range cases {
		if got := detail(tc.a, now); got != tc.want {
			t.Errorf("%s: detail = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// A forward whose pod refuses connections stays running; the DETAIL column
// says what failed and how long ago, and keeps counting.
func TestATunnelErrorShowsWithItsAge(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	failedAt := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	mgr, err := forwards.NewManager(nil, forwards.WithTunneler(&refusingTunneler{failedAt: failedAt}))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	t.Cleanup(func() { _ = mgr.StopAll() })
	f := forwards.Forward{ID: forwards.NewID(), Context: "fake", Namespace: "demo", Kind: "pod", Name: "web", LocalPort: freeLocalPort(t), RemotePort: 80}
	if err := mgr.Register(f); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(t.Context(), f.ID); err != nil {
		t.Fatalf("Start: %v", err)
	}

	now := failedAt.Add(12 * time.Second)
	m := New(mgr, WithClock(func() time.Time { return now }))
	t.Cleanup(func() { _ = m.Close() })
	next, _ := m.Update(tea.WindowSizeMsg{Width: 140, Height: 10})
	m = next.(Model)
	if got := screen(m); !strings.Contains(got, "running") || !strings.Contains(got, "last error: connection refused · 12s ago") {
		t.Fatalf("screen:\n%s", got)
	}
	if strings.Contains(m.View(), "\x1b]0;") {
		t.Error("the tunnel error reached the screen unsanitized")
	}
	if m.Init() == nil {
		t.Fatal("Init armed nothing")
	}

	now = failedAt.Add(75 * time.Second)
	next, cmd := m.Update(ageTickMsg{tok: m.ageTok})
	m = next.(Model)
	if got := screen(m); !strings.Contains(got, "· 75s ago") {
		t.Errorf("the age did not count on:\n%s", got)
	}
	if cmd == nil {
		t.Error("the age stopped counting while the error is shown")
	}
	if _, stale := m.Update(ageTickMsg{tok: m.ageTok.Next()}); stale != nil {
		t.Error("a tick of another chain was taken")
	}

	if err := mgr.Stop(f.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	next, cmd = m.Update(ageTickMsg{tok: m.ageTok})
	m = next.(Model)
	if cmd != nil {
		t.Error("the age keeps ticking with no error shown")
	}
	if got := screen(m); strings.Contains(got, "last error") {
		t.Errorf("a stopped forward still shows its tunnel error:\n%s", got)
	}
}
