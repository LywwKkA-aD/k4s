package forwards

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStartReachesRunning(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		kind       string
		wantPod    string
		wantRemote uint16
	}{
		{name: "pod forwards to itself", kind: "pod", wantPod: "web", wantRemote: 80},
		{name: "deployment forwards to its resolved pod", kind: "deployment", wantPod: "web-pod", wantRemote: 80},
		{name: "service forwards to the target port", kind: "service", wantPod: "web-pod", wantRemote: 8080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeTunneler(t)
			m := newTestManager(t, deps{tunnel: fake})
			mustRegister(t, m, testForward("f1", tt.kind, 18080))

			changed := m.Changes()
			done := startAsync(context.Background(), m, "f1")
			sess := fake.next(t)

			if got := statusOf(t, m, "f1").Status; got != StatusStarting {
				t.Errorf("status before ready = %v, want %v", got, StatusStarting)
			}
			select {
			case <-changed:
			default:
				t.Error("Start did not signal a change when it began")
			}

			sess.ready()
			if err := awaitErr(t, done); err != nil {
				t.Fatalf("Start: %v", err)
			}
			a := statusOf(t, m, "f1")
			if a.Status != StatusRunning || a.Err != nil {
				t.Errorf("after ready = %v (err: %v), want running without error", a.Status, a.Err)
			}
			want := startCall{namespace: testNamespace, pod: tt.wantPod, local: 18080, remote: tt.wantRemote}
			if got := fake.lastCall(t); got != want {
				t.Errorf("StartPodPortForward got %+v, want %+v", got, want)
			}
		})
	}
}

func TestStartFailsBeforeReady(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		// kill ends the session before it is ready.
		kill    func(s *fakeSession)
		wantErr string
		// wantClosed: a session that reported an error is closed; one
		// that already exited has nothing left to close.
		wantClosed bool
	}{
		{
			name: "forwarder error carries stderr detail",
			kill: func(s *fakeSession) {
				s.writeStderr("E1021 portforward.go:413] bind: address already in use\n")
				s.Err <- errors.New("lost connection to pod")
			},
			wantErr:    "lost connection to pod: bind: address already in use",
			wantClosed: true,
		},
		{
			name:    "forwarder exits without error",
			kill:    func(s *fakeSession) { s.finish() },
			wantErr: "forward terminated before ready",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeTunneler(t)
			m := newTestManager(t, deps{tunnel: fake})
			mustRegister(t, m, testForward("f1", "pod", 18080))

			done := startAsync(context.Background(), m, "f1")
			sess := fake.next(t)
			tt.kill(sess)

			assertMarked(t, m, awaitErr(t, done), tt.wantErr)
			if tt.wantClosed {
				sess.waitStopped(t)
			}
		})
	}
}

func TestStartTimesOutWaitingForReady(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))

	done := startAsync(timeoutCtx(t, 20*time.Millisecond), m, "f1")
	sess := fake.next(t)

	err := awaitErr(t, done)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start error = %v, want %v", err, context.DeadlineExceeded)
	}
	assertMarked(t, m, err, context.DeadlineExceeded.Error())
	sess.waitStopped(t)
}

func TestStartFailsBeforeTunnel(t *testing.T) {
	t.Parallel()
	busy := errors.New("listen tcp 127.0.0.1:18080: bind: address already in use")
	tests := []struct {
		name       string
		kind       string
		noClient   bool
		probeErr   error
		startErr   error
		resolveErr error
		wantErr    string
	}{
		{name: "no kubernetes client", kind: "pod", noClient: true, wantErr: "no kubernetes client"},
		{name: "local port busy", kind: "pod", probeErr: busy, wantErr: "local port 18080 already in use"},
		{name: "target does not resolve", kind: "deployment", resolveErr: errors.New("no ready pod"), wantErr: "resolve target: no ready pod"},
		{name: "tunnel does not open", kind: "pod", startErr: errors.New("no rest config"), wantErr: "start forward: no rest config"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeTunneler(t)
			fake.startErr = tt.startErr
			if tt.resolveErr != nil {
				fake.resolveDeployment = func(context.Context, string) (string, error) { return "", tt.resolveErr }
			}
			d := deps{tunnel: fake, probePort: func(uint16) error { return tt.probeErr }}
			if tt.noClient {
				d.tunnel = nil
			}
			m := newTestManager(t, d)
			mustRegister(t, m, testForward("f1", tt.kind, 18080))

			assertMarked(t, m, m.Start(context.Background(), "f1"), tt.wantErr)
		})
	}
}

func TestStartRefusesUnknownAndLiveForwards(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))

	assertErrContains(t, m.Start(context.Background(), "missing"), `forward "missing" not found`)

	done := startAsync(context.Background(), m, "f1")
	sess := fake.next(t)
	assertErrContains(t, m.Start(context.Background(), "f1"), "already running")

	sess.ready()
	if err := awaitErr(t, done); err != nil {
		t.Fatalf("Start: %v", err)
	}
	assertErrContains(t, m.Start(context.Background(), "f1"), "already running")
}

func TestStopWhileStarting(t *testing.T) {
	t.Parallel()

	t.Run("while resolving the target", func(t *testing.T) {
		t.Parallel()
		fake := newFakeTunneler(t)
		g := newGate(t)
		fake.resolveDeployment = g.hook()
		m := newTestManager(t, deps{tunnel: fake})
		mustRegister(t, m, testForward("f1", "deployment", 18080))

		done := startAsync(context.Background(), m, "f1")
		g.wait(t)
		if err := m.Stop("f1"); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		g.release <- nil

		sess := fake.next(t)
		if err := awaitErr(t, done); err != nil {
			t.Errorf("Start after Stop = %v, want nil", err)
		}
		sess.waitStopped(t)
		a := statusOf(t, m, "f1")
		if a.Status != StatusStopped || a.Err != nil {
			t.Errorf("after Stop = %v (err: %v), want stopped without error", a.Status, a.Err)
		}
	})

	t.Run("while waiting for ready", func(t *testing.T) {
		t.Parallel()
		fake := newFakeTunneler(t)
		m := newTestManager(t, deps{tunnel: fake})
		mustRegister(t, m, testForward("f1", "pod", 18080))

		done := startAsync(context.Background(), m, "f1")
		sess := fake.next(t)
		// Stop may land just before or just after Start stores the
		// session; either way the session must end up closed.
		if err := m.Stop("f1"); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		sess.waitStopped(t)
		_ = awaitErr(t, done)
		a := statusOf(t, m, "f1")
		if a.Status != StatusStopped || a.Err != nil {
			t.Errorf("after Stop = %v (err: %v), want stopped without error", a.Status, a.Err)
		}
	})
}

func TestRestartWhileStarting(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))

	first := startAsync(context.Background(), m, "f1")
	old := fake.next(t)
	if err := m.Stop("f1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	second := startAsync(context.Background(), m, "f1")
	cur := fake.next(t)
	_ = awaitErr(t, first)

	cur.ready()
	if err := awaitErr(t, second); err != nil {
		t.Fatalf("second Start: %v", err)
	}
	a := statusOf(t, m, "f1")
	if a.Status != StatusRunning || a.Err != nil {
		t.Errorf("after restart = %v (err: %v), want running without error", a.Status, a.Err)
	}
	old.waitStopped(t)
	if cur.isStopped() {
		t.Error("the restarted session was closed")
	}
}

func TestSuperviseAfterLostConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		end        func(s *fakeSession)
		wantStatus Status
		wantErr    string
	}{
		{
			name: "forwarder error",
			end: func(s *fakeSession) {
				s.Err <- errors.New("lost connection to pod")
				s.finish()
			},
			wantStatus: StatusError,
			wantErr:    "lost connection to pod",
		},
		{
			name: "stderr without error",
			end: func(s *fakeSession) {
				s.writeStderr("Unable to listen on port 18080\nbind: permission denied\n")
				s.finish()
			},
			wantStatus: StatusError,
			wantErr:    "bind: permission denied",
		},
		{
			name:       "clean exit",
			end:        func(s *fakeSession) { s.finish() },
			wantStatus: StatusStopped,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fake := newFakeTunneler(t)
			m := newTestManager(t, deps{tunnel: fake})
			mustRegister(t, m, testForward("f1", "pod", 18080))

			done := startAsync(context.Background(), m, "f1")
			sess := fake.next(t)
			sess.ready()
			if err := awaitErr(t, done); err != nil {
				t.Fatalf("Start: %v", err)
			}

			tt.end(sess)
			a := waitStatus(t, m, "f1", tt.wantStatus)
			if tt.wantErr == "" {
				if a.Err != nil {
					t.Errorf("err = %v, want nil", a.Err)
				}
				return
			}
			assertErrContains(t, a.Err, tt.wantErr)
		})
	}
}
