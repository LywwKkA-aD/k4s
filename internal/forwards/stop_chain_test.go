package forwards

import (
	"errors"
	"testing"
	"time"
)

func TestBeginStopAllOutlastsTheManagersOwnPendingStop(t *testing.T) {
	t.Parallel()
	first, sessions := threeRunning(t, deps{stopWait: testTimeout})
	firstStop := first.BeginStopAll()

	// The second manager has no sessions of its own, yet its stop is not
	// over while the first one still runs.
	second := newTestManager(t, deps{after: firstStop})
	secondStop := second.BeginStopAll()
	third := newTestManager(t, deps{after: secondStop})
	if secondStop.Count() != 0 {
		t.Errorf("Count = %d, want 0 own sessions", secondStop.Count())
	}
	if err := third.Blocked(); !errors.Is(err, ErrStopping) {
		t.Fatalf("Blocked = %v, want ErrStopping while the first stop runs", err)
	}
	select {
	case <-secondStop.Done():
		t.Fatal("the second stop ended before the first")
	case <-time.After(50 * time.Millisecond):
	}

	for _, s := range sessions {
		s.finish()
	}
	if err := secondStop.Wait(); err != nil {
		t.Errorf("Wait = %v, want nil", err)
	}
	if err := third.Blocked(); err != nil {
		t.Errorf("Blocked after the first stop ended = %v, want nil", err)
	}
}

func TestBeginStopAllChainEndsWhenTheFirstStopTimesOut(t *testing.T) {
	t.Parallel()
	first, _ := threeRunning(t, deps{stopWait: stopWait})
	firstStop := first.BeginStopAll()
	second := newTestManager(t, deps{after: firstStop})
	secondStop := second.BeginStopAll()

	if err := secondStop.Wait(); err != nil {
		t.Errorf("second Wait = %v, want nil: the timeout belongs to the first stop", err)
	}
	if !errors.Is(firstStop.Wait(), ErrStopTimeout) {
		t.Error("the first stop did not report its timeout")
	}
}
