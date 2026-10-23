package forwards

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// panicLog records what the Manager logs about a panic.
type panicLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *panicLog) logf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

func (l *panicLog) text() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Join(l.lines, "\n")
}

// A panic in the supervisor must not take the process down with the
// terminal in raw mode: the forward shows an internal error, the stack goes
// to the k4s log, and the Manager keeps working.
func TestASupervisorPanicBecomesAnInternalError(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	log := &panicLog{}
	m := newTestManager(t, deps{tunnel: fake, logf: log.logf})
	mustRegister(t, m, testForward("f1", "pod", 18080), testForward("f2", "pod", 18081))
	sess := running(t, m, fake, "f1")[0]

	sess.panicOnStderr.Store(true)
	sess.finish()
	a := waitStatus(t, m, "f1", StatusError)
	if !errors.Is(a.Err, k8s.ErrInternal) || !strings.HasPrefix(a.Err.Error(), "internal error (see ") ||
		!strings.HasSuffix(a.Err.Error(), "k4s.log)") {
		t.Errorf("Err = %v, want an internal error pointing at the k4s log", a.Err)
	}
	if got := log.text(); !strings.Contains(got, "stderr capture broke") || !strings.Contains(got, "goroutine") {
		t.Errorf("the log lacks the panic and its stack:\n%s", got)
	}

	running(t, m, fake, "f2")
	if err := m.Remove("f1"); err != nil {
		t.Errorf("Remove after the panic: %v", err)
	}
}

// The forwarder goroutine reports a panic as k8s.ErrInternal; the forward
// shows the same internal error.
func TestAForwarderPanicIsAnInternalError(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))
	done := startAsync(context.Background(), m, "f1")
	sess := fake.next(t)
	sess.Err <- fmt.Errorf("%w: nil map", k8s.ErrInternal)
	sess.finish()

	err := awaitErr(t, done)
	if !errors.Is(err, k8s.ErrInternal) || !strings.Contains(err.Error(), "(see ") {
		t.Errorf("Start = %v, want the internal error", err)
	}
}

func TestLogPath(t *testing.T) {
	t.Setenv("HOME", "/home/dev")
	cases := []struct {
		stateHome, want string
	}{
		{stateHome: "", want: filepath.Join("~", ".local", "state", "k4s", "k4s.log")},
		{stateHome: "relative", want: filepath.Join("~", ".local", "state", "k4s", "k4s.log")},
		{stateHome: "/var/state", want: filepath.Join("/var/state", "k4s", "k4s.log")},
	}
	for _, tc := range cases {
		t.Setenv("XDG_STATE_HOME", tc.stateHome)
		if got := logPath(); got != tc.want {
			t.Errorf("XDG_STATE_HOME=%q: logPath = %q, want %q", tc.stateHome, got, tc.want)
		}
	}
}

// panicTunneler panics as it opens a tunnel, the way a bug in client-go's
// dial or in k4s's resolve would.
type panicTunneler struct{}

func (panicTunneler) StartPodPortForward(context.Context, string, string, uint16, uint16) (*k8s.PortForwardSession, error) {
	panic("dial broke")
}

func (panicTunneler) ResolveServiceToPod(context.Context, string, string, int32) (*corev1.Pod, int32, error) {
	panic("resolve broke")
}

func (panicTunneler) ResolveDeploymentToPod(context.Context, string, string) (string, error) {
	panic("resolve broke")
}

// Start runs on a goroutine Bubble Tea does not guard: a panic while it
// opens the tunnel ends that start with an internal error on the forward
// and the stack in the k4s log, not the process.
func TestAStartPanicBecomesAnInternalError(t *testing.T) {
	t.Parallel()
	log := &panicLog{}
	m := newTestManager(t, deps{tunnel: panicTunneler{}, logf: log.logf})
	mustRegister(t, m, testForward("f1", "pod", 18090))

	err := m.Start(context.Background(), "f1")
	if !errors.Is(err, k8s.ErrInternal) {
		t.Fatalf("Start = %v, want the internal error", err)
	}
	if a := waitStatus(t, m, "f1", StatusError); !errors.Is(a.Err, k8s.ErrInternal) {
		t.Errorf("forward Err = %v, want the internal error", a.Err)
	}
	if got := log.text(); !strings.Contains(got, "dial broke") || !strings.Contains(got, "goroutine") {
		t.Errorf("the log lacks the panic and its stack:\n%s", got)
	}
}
