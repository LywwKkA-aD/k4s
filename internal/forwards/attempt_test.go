package forwards

import (
	"context"
	"errors"
	"testing"
)

// A Start parked before it has a session must not touch the forward once a
// newer Start has taken over: neither its failure nor its late session may
// replace what the newer attempt reports.
func TestStaleAttemptCannotOverrideNewerStart(t *testing.T) {
	t.Parallel()

	t.Run("late failure after the restart is running", func(t *testing.T) {
		t.Parallel()
		fake := newFakeTunneler(t)
		g := newGate(t)
		fake.resolveDeployment = g.hook()
		m := newTestManager(t, deps{tunnel: fake})
		mustRegister(t, m, testForward("f1", "deployment", 18080))

		stale := startAsync(context.Background(), m, "f1")
		g.wait(t)
		if err := m.Stop("f1"); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		fresh := startAsync(context.Background(), m, "f1")
		sess := fake.next(t)
		sess.ready()
		if err := awaitErr(t, fresh); err != nil {
			t.Fatalf("second Start: %v", err)
		}

		g.release <- errors.New("no ready pod for deployment \"web\"")
		_ = awaitErr(t, stale)

		a := statusOf(t, m, "f1")
		if a.Status != StatusRunning || a.Err != nil {
			t.Errorf("after the stale failure = %v (err: %v), want running without error", a.Status, a.Err)
		}
	})

	t.Run("late failure after remove and re-register with the same id", func(t *testing.T) {
		t.Parallel()
		fake := newFakeTunneler(t)
		g := newGate(t)
		fake.resolveDeployment = g.hook()
		m := newTestManager(t, deps{tunnel: fake})
		mustRegister(t, m, testForward("f1", "deployment", 18080))

		stale := startAsync(context.Background(), m, "f1")
		g.wait(t)
		if err := m.Remove("f1"); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		mustRegister(t, m, testForward("f1", "pod", 18080))
		fresh := startAsync(context.Background(), m, "f1")
		sess := fake.next(t)
		sess.ready()
		if err := awaitErr(t, fresh); err != nil {
			t.Fatalf("Start after re-register: %v", err)
		}

		g.release <- errors.New("no ready pod for deployment \"web\"")
		_ = awaitErr(t, stale)

		a := statusOf(t, m, "f1")
		if a.Status != StatusRunning || a.Err != nil {
			t.Errorf("after the stale failure = %v (err: %v), want running without error", a.Status, a.Err)
		}
	})

	t.Run("late session while the restart waits for ready", func(t *testing.T) {
		t.Parallel()
		fake := newFakeTunneler(t)
		g := newGate(t)
		fake.resolveDeployment = g.hook()
		m := newTestManager(t, deps{tunnel: fake})
		mustRegister(t, m, testForward("f1", "deployment", 18080))

		stale := startAsync(timeoutCtx(t, testTimeout), m, "f1")
		g.wait(t)
		if err := m.Stop("f1"); err != nil {
			t.Fatalf("Stop: %v", err)
		}
		fresh := startAsync(context.Background(), m, "f1")
		cur := fake.next(t)

		g.release <- nil
		old := fake.next(t)
		if err := awaitErr(t, stale); err != nil {
			t.Errorf("stale Start = %v, want nil once it gives way", err)
		}
		old.waitStopped(t)

		cur.ready()
		if err := awaitErr(t, fresh); err != nil {
			t.Fatalf("second Start: %v", err)
		}
		a := statusOf(t, m, "f1")
		if a.Status != StatusRunning || a.Err != nil {
			t.Errorf("after the restart = %v (err: %v), want running without error", a.Status, a.Err)
		}
		if cur.isStopped() {
			t.Error("the newer session was closed")
		}
	})
}
