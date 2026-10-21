package forwards

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// testTimeout bounds every wait in these tests. It is generous on purpose:
// the suite must stay stable under -race -count=50.
const testTimeout = 5 * time.Second

const (
	testContext   = "dev"
	testNamespace = "k4s-demo"
)

// fakeSession is a PortForwardSession whose channels the test drives. Like
// the real forwarder, it closes Done once StopCh is closed.
type fakeSession struct {
	*k8s.PortForwardSession
	doneOnce sync.Once
	mu       sync.Mutex
	stderr   string
}

// newFakeSession returns a session that winds down once stopped, or, when
// stuck, one that ignores StopCh until the test calls finish.
func newFakeSession(pod string, stuck bool) *fakeSession {
	s := &fakeSession{}
	s.PortForwardSession = &k8s.PortForwardSession{
		StopCh: make(chan struct{}),
		Ready:  make(chan struct{}),
		Err:    make(chan error, 1),
		Done:   make(chan struct{}),
		Pod:    pod,
		Stderr: s.stderrText,
		Stdout: func() string { return "" },
	}
	if stuck {
		return s
	}
	go func() {
		select {
		case <-s.StopCh:
			s.finish()
		case <-s.Done:
		}
	}()
	return s
}

// finish ends the session the way a returning ForwardPorts does.
func (s *fakeSession) finish() { s.doneOnce.Do(func() { close(s.Done) }) }

func (s *fakeSession) ready() { close(s.Ready) }

func (s *fakeSession) writeStderr(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stderr += text
}

func (s *fakeSession) stderrText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr
}

// waitStopped fails the test unless the Manager closes StopCh in time.
func (s *fakeSession) waitStopped(t *testing.T) {
	t.Helper()
	select {
	case <-s.StopCh:
	case <-time.After(testTimeout):
		t.Fatalf("session for pod %q was never stopped", s.Pod)
	}
}

func (s *fakeSession) isStopped() bool {
	select {
	case <-s.StopCh:
		return true
	default:
		return false
	}
}

// startCall records one StartPodPortForward invocation.
type startCall struct {
	namespace, pod string
	local, remote  uint16
}

// fakeTunneler hands out fakeSessions on a channel so a test can grab the
// session a Start created and decide its fate. The hook fields are set
// before the Manager sees the fake and are read-only afterwards.
type fakeTunneler struct {
	sessions chan *fakeSession
	startErr error
	// stuck makes every session ignore StopCh, like a forwarder whose
	// connection hangs on close.
	stuck bool
	// resolveDeployment replaces the default "<name>-pod" answer.
	resolveDeployment func(ctx context.Context, name string) (string, error)

	mu    sync.Mutex
	calls []startCall
	all   []*fakeSession
}

func newFakeTunneler(t *testing.T) *fakeTunneler {
	t.Helper()
	f := &fakeTunneler{sessions: make(chan *fakeSession, 16)}
	t.Cleanup(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		for _, s := range f.all {
			s.finish()
		}
	})
	return f
}

func (f *fakeTunneler) StartPodPortForward(_ context.Context, namespace, podName string, localPort, remotePort uint16) (*k8s.PortForwardSession, error) {
	if f.startErr != nil {
		return nil, f.startErr
	}
	s := newFakeSession(podName, f.stuck)
	f.mu.Lock()
	f.calls = append(f.calls, startCall{namespace: namespace, pod: podName, local: localPort, remote: remotePort})
	f.all = append(f.all, s)
	f.mu.Unlock()
	f.sessions <- s
	return s.PortForwardSession, nil
}

func (f *fakeTunneler) ResolveServiceToPod(_ context.Context, _, serviceName string, svcPort int32) (*corev1.Pod, int32, error) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: serviceName + "-pod"}}
	return pod, svcPort + 8000, nil
}

func (f *fakeTunneler) ResolveDeploymentToPod(ctx context.Context, _, deployName string) (string, error) {
	if f.resolveDeployment != nil {
		return f.resolveDeployment(ctx, deployName)
	}
	return deployName + "-pod", nil
}

func (f *fakeTunneler) lastCall(t *testing.T) startCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("StartPodPortForward was never called")
	}
	return f.calls[len(f.calls)-1]
}

func (f *fakeTunneler) next(t *testing.T) *fakeSession {
	t.Helper()
	select {
	case s := <-f.sessions:
		return s
	case <-time.After(testTimeout):
		t.Fatal("no port-forward session was opened")
		return nil
	}
}

// gate makes a resolve hook block until the test releases it, so a Start
// can be parked before it has a session.
type gate struct {
	entered chan struct{}
	release chan error
}

func newGate(t *testing.T) gate {
	t.Helper()
	g := gate{entered: make(chan struct{}, 1), release: make(chan error, 1)}
	t.Cleanup(func() {
		select {
		case g.release <- nil:
		default:
		}
	})
	return g
}

// hook returns a resolveDeployment that parks its first call until the
// test releases it with an error (or nil for success). Later calls answer
// at once.
func (g gate) hook() func(context.Context, string) (string, error) {
	var calls atomic.Int32
	return func(_ context.Context, name string) (string, error) {
		if calls.Add(1) == 1 {
			g.entered <- struct{}{}
			if err := <-g.release; err != nil {
				return "", err
			}
		}
		return name + "-pod", nil
	}
}

func (g gate) wait(t *testing.T) {
	t.Helper()
	select {
	case <-g.entered:
	case <-time.After(testTimeout):
		t.Fatal("resolve was never called")
	}
}

// newTestManager fills the deps a test leaves zero: a probe that always
// finds the port free and a state file under t.TempDir.
func newTestManager(t *testing.T, d deps) *Manager {
	t.Helper()
	if d.probePort == nil {
		d.probePort = func(uint16) error { return nil }
	}
	if d.statePath == "" {
		d.statePath = filepath.Join(t.TempDir(), "portforwards.json")
	}
	if d.cluster == (cluster{}) {
		d.cluster = cluster{contextName: testContext, server: "https://127.0.0.1:6443"}
	}
	m, err := newManager(d)
	if err != nil {
		t.Fatalf("newManager: %v", err)
	}
	return m
}

func testForward(id, kind string, local uint16) Forward {
	return Forward{
		ID:         id,
		Context:    testContext,
		Namespace:  testNamespace,
		Kind:       kind,
		Name:       "web",
		LocalPort:  local,
		RemotePort: 80,
	}
}

func mustRegister(t *testing.T, m *Manager, fwds ...Forward) {
	t.Helper()
	for _, f := range fwds {
		if err := m.Register(f); err != nil {
			t.Fatalf("Register(%s): %v", f.ID, err)
		}
	}
}

func startAsync(ctx context.Context, m *Manager, id string) <-chan error {
	out := make(chan error, 1)
	go func() { out <- m.Start(ctx, id) }()
	return out
}

func awaitErr(t *testing.T, ch <-chan error) error {
	t.Helper()
	select {
	case err := <-ch:
		return err
	case <-time.After(testTimeout):
		t.Fatal("Start did not return")
		return nil
	}
}

func find(list []Active, id string) (Active, bool) {
	for _, a := range list {
		if a.Forward.ID == id {
			return a, true
		}
	}
	return Active{}, false
}

func statusOf(t *testing.T, m *Manager, id string) Active {
	t.Helper()
	a, ok := find(m.List(), id)
	if !ok {
		t.Fatalf("forward %s is not listed", id)
	}
	return a
}

// waitStatus re-reads List on every change notification until the forward
// reaches want, so the test never sleeps.
func waitStatus(t *testing.T, m *Manager, id string, want Status) Active {
	t.Helper()
	deadline := time.After(testTimeout)
	for {
		changed := m.Changes()
		if a, ok := find(m.List(), id); ok && a.Status == want {
			return a
		}
		select {
		case <-changed:
		case <-deadline:
			a, _ := find(m.List(), id)
			t.Fatalf("forward %s status = %v (err: %v), want %v", id, a.Status, a.Err, want)
		}
	}
}

// assertMarked checks that forward f1 is in error and that Start returned
// exactly the message the status column shows.
func assertMarked(t *testing.T, m *Manager, returned error, want string) {
	t.Helper()
	a := statusOf(t, m, "f1")
	if a.Status != StatusError {
		t.Errorf("status = %v, want %v", a.Status, StatusError)
	}
	if returned == nil || a.Err == nil {
		t.Fatalf("Start returned %v and marked %v, want both to be %q", returned, a.Err, want)
	}
	if returned.Error() != want || a.Err.Error() != want {
		t.Errorf("Start returned %q and marked %q, want both to be %q", returned, a.Err, want)
	}
}

func assertErrContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want it to contain %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), want)
	}
}

func timeoutCtx(t *testing.T, d time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	t.Cleanup(cancel)
	return ctx
}
