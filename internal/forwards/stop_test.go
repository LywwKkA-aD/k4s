package forwards

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// stopWait is the deadline the stop tests give a Manager: long enough that a
// session winding down on its own beats it under -race, short enough that a
// hung one does not slow the suite.
const stopWait = 300 * time.Millisecond

// running starts each forward through fake and returns its session once the
// forward is running.
func running(t *testing.T, m *Manager, fake *fakeTunneler, ids ...string) []*fakeSession {
	t.Helper()
	out := make([]*fakeSession, 0, len(ids))
	for _, id := range ids {
		done := startAsync(context.Background(), m, id)
		s := fake.next(t)
		s.ready()
		if err := awaitErr(t, done); err != nil {
			t.Fatalf("Start(%s): %v", id, err)
		}
		out = append(out, s)
	}
	return out
}

// windDownOnStop makes a stuck session behave: it finishes once stopped.
func windDownOnStop(s *fakeSession) {
	go func() {
		<-s.StopCh
		s.finish()
	}()
}

func threeRunning(t *testing.T, d deps) (*Manager, []*fakeSession) {
	t.Helper()
	fake := newFakeTunneler(t)
	fake.stuck = true
	d.tunnel = fake
	m := newTestManager(t, d)
	mustRegister(t, m,
		testForward("a", "pod", 18081),
		testForward("b", "pod", 18082),
		testForward("c", "pod", 18083),
	)
	return m, running(t, m, fake, "a", "b", "c")
}

func TestStopAllWaitsInParallelUnderOneDeadline(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		hung    int
		wantErr string
	}{
		{name: "every session winds down", hung: 0},
		{name: "one session hangs", hung: 1, wantErr: "stop timed out: 1 of 3 forwards still closing"},
		{name: "every session hangs", hung: 3, wantErr: "stop timed out: 3 of 3 forwards still closing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, sessions := threeRunning(t, deps{stopWait: stopWait})
			for _, s := range sessions[tt.hung:] {
				windDownOnStop(s)
			}

			changed := m.Changes()
			begin := time.Now()
			err := m.StopAll()
			took := time.Since(begin)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("StopAll = %v, want nil", err)
				}
			} else {
				if !errors.Is(err, ErrStopTimeout) {
					t.Fatalf("StopAll = %v, want ErrStopTimeout", err)
				}
				if err.Error() != tt.wantErr {
					t.Errorf("StopAll = %q, want %q", err, tt.wantErr)
				}
				if took < stopWait {
					t.Errorf("StopAll gave up after %v, before the %v deadline", took, stopWait)
				}
			}
			// Three hung sessions waited one after another would take three
			// deadlines.
			if took >= 2*stopWait {
				t.Errorf("StopAll took %v, want one %v deadline for all sessions", took, stopWait)
			}
			select {
			case <-changed:
			default:
				t.Error("StopAll did not signal a change")
			}
			for _, s := range sessions {
				if !s.isStopped() {
					t.Errorf("session for %q was not closed", s.Pod)
				}
			}
			for _, id := range []string{"a", "b", "c"} {
				if a := statusOf(t, m, id); a.Status != StatusStopped || a.Err != nil {
					t.Errorf("%s after StopAll = %v (err: %v), want stopped without error", id, a.Status, a.Err)
				}
			}
		})
	}
}

func TestStopAllWithoutSessions(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, deps{})
	mustRegister(t, m, testForward("f1", "pod", 18080))
	stop := m.BeginStopAll()
	if stop.Count() != 0 {
		t.Errorf("Count = %d, want 0", stop.Count())
	}
	select {
	case <-stop.Done():
	default:
		t.Fatal("a stop with nothing to close is not done at once")
	}
	if err := stop.Wait(); err != nil {
		t.Errorf("Wait = %v, want nil", err)
	}
}

func TestBeginStopAllReturnsBeforeTheSessionsEnd(t *testing.T) {
	t.Parallel()
	m, sessions := threeRunning(t, deps{stopWait: testTimeout})

	stop := m.BeginStopAll()
	if stop.Count() != 3 {
		t.Errorf("Count = %d, want 3", stop.Count())
	}
	for _, s := range sessions {
		if !s.isStopped() {
			t.Errorf("session for %q was not closed before BeginStopAll returned", s.Pod)
		}
	}
	select {
	case <-stop.Done():
		t.Fatal("the stop is done while every session is still open")
	default:
	}
	if a := statusOf(t, m, "a"); a.Status != StatusStopped {
		t.Errorf("status = %v, want stopped at once", a.Status)
	}

	for _, s := range sessions {
		s.finish()
	}
	if err := stop.Wait(); err != nil {
		t.Errorf("Wait = %v, want nil once every session ended", err)
	}
}

func TestStopTimesOutOnAHungSession(t *testing.T) {
	t.Parallel()
	m, _ := threeRunning(t, deps{stopWait: stopWait})

	err := m.Stop("a")
	if !errors.Is(err, ErrStopTimeout) {
		t.Fatalf("Stop = %v, want ErrStopTimeout", err)
	}
	if a := statusOf(t, m, "a"); a.Status != StatusStopped || a.Err != nil {
		t.Errorf("after Stop = %v (err: %v), want stopped without error", a.Status, a.Err)
	}
	if !errors.Is(m.Stop("missing"), ErrNotFound) {
		t.Error("Stop of an unknown id is not ErrNotFound")
	}
}

func TestRemoveReportsAHungStop(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "portforwards.json")
	m, _ := threeRunning(t, deps{stopWait: stopWait, statePath: path})

	if err := m.Remove("a"); !errors.Is(err, ErrStopTimeout) {
		t.Fatalf("Remove = %v, want ErrStopTimeout", err)
	}
	if _, ok := find(m.List(), "a"); ok {
		t.Error("the forward is still listed")
	}
	onDisk, err := loadFrom(path)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if _, ok := onDisk.FindByID("a"); ok {
		t.Error("the forward is still saved")
	}
}

func TestAfterHoldsRegisterAndStartBack(t *testing.T) {
	t.Parallel()
	old, sessions := threeRunning(t, deps{stopWait: testTimeout})
	stop := old.BeginStopAll()

	fake := newFakeTunneler(t)
	path := filepath.Join(t.TempDir(), "portforwards.json")
	saved := testForward("saved", "pod", 18090)
	seed := newTestManager(t, deps{statePath: path})
	mustRegister(t, seed, saved)
	next := newTestManager(t, deps{tunnel: fake, statePath: path, after: stop})

	if err := next.Blocked(); !errors.Is(err, ErrStopping) {
		t.Fatalf("Blocked = %v, want ErrStopping", err)
	}
	if err := next.Register(testForward("new", "pod", 18081)); !errors.Is(err, ErrStopping) {
		t.Errorf("Register = %v, want ErrStopping", err)
	}
	if _, ok := find(next.List(), "new"); ok {
		t.Error("a refused Register was listed")
	}
	if err := next.Start(context.Background(), "saved"); !errors.Is(err, ErrStopping) {
		t.Errorf("Start = %v, want ErrStopping", err)
	}
	if a := statusOf(t, next, "saved"); a.Status != StatusStopped || a.Err != nil {
		t.Errorf("a refused Start left %v (err: %v), want stopped without error", a.Status, a.Err)
	}

	for _, s := range sessions {
		s.finish()
	}
	if err := stop.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if err := next.Blocked(); err != nil {
		t.Errorf("Blocked after the stop = %v, want nil", err)
	}
	mustRegister(t, next, testForward("new", "pod", 18081))
	_ = running(t, next, fake, "new")
}

func TestAfterLetsGoOnTimeout(t *testing.T) {
	t.Parallel()
	old, _ := threeRunning(t, deps{stopWait: stopWait})
	stop := old.BeginStopAll()
	next := newTestManager(t, deps{after: stop})

	if !errors.Is(stop.Wait(), ErrStopTimeout) {
		t.Fatal("the hung stop did not time out")
	}
	if err := next.Blocked(); err != nil {
		t.Errorf("Blocked after the timeout = %v, want nil", err)
	}
}

func TestAfterWithNothingToWaitFor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		after *Stopping
	}{
		{name: "no previous manager", after: nil},
		{name: "nothing was running", after: newTestManager(t, deps{}).BeginStopAll()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newTestManager(t, deps{after: tt.after})
			if err := m.Blocked(); err != nil {
				t.Errorf("Blocked = %v, want nil", err)
			}
			mustRegister(t, m, testForward("f1", "pod", 18080))
		})
	}
}

func TestDefaultStopDeadline(t *testing.T) {
	t.Parallel()
	if m := newTestManager(t, deps{}); m.stopWait != 2*time.Second {
		t.Errorf("stop deadline = %v, want 2s", m.stopWait)
	}
}
