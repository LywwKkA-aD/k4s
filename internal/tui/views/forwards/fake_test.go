package forwards_test

import (
	"context"
	"net"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// fakeTunneler opens sessions that are ready at once. A stuck one ignores
// StopCh, like a forwarder whose connection hangs on close; every session
// ends when the test does.
type fakeTunneler struct {
	stuck bool

	mu   sync.Mutex
	ends []func()
}

func newFakeTunneler(t *testing.T, stuck bool) *fakeTunneler {
	t.Helper()
	f := &fakeTunneler{stuck: stuck}
	t.Cleanup(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, end := range f.ends {
			end()
		}
	})
	return f
}

func (f *fakeTunneler) StartPodPortForward(_ context.Context, _, pod string, _, _ uint16) (*k8s.PortForwardSession, error) {
	s := &k8s.PortForwardSession{
		StopCh: make(chan struct{}),
		Ready:  make(chan struct{}),
		Err:    make(chan error, 1),
		Done:   make(chan struct{}),
		Pod:    pod,
		Stderr: func() string { return "" },
	}
	close(s.Ready)
	var once sync.Once
	end := func() { once.Do(func() { close(s.Done) }) }
	if !f.stuck {
		go func() {
			<-s.StopCh
			end()
		}()
	}
	f.mu.Lock()
	f.ends = append(f.ends, end)
	f.mu.Unlock()
	return s, nil
}

func (f *fakeTunneler) ResolveServiceToPod(_ context.Context, _, name string, port int32) (*corev1.Pod, int32, error) {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name + "-pod"}}, port, nil
}

func (f *fakeTunneler) ResolveDeploymentToPod(_ context.Context, _, name string) (string, error) {
	return name + "-pod", nil
}

// freePort returns a local port nothing listens on right now: the Manager
// probes the real port before it starts a forward.
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
