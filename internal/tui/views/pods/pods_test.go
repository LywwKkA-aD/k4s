package pods

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// tickBatchSize reports how many commands the tick handler bundled.
// tea.Batch returns a lone command unwrapped (not as BatchMsg), and the only
// lone command here is the 5s ticker — so "no immediate result" means the
// fetch was skipped and only the ticker was re-armed. Sub-commands are never
// invoked: the fetch would hit the API, the ticker would sleep.
func tickBatchSize(t *testing.T, cmd tea.Cmd) int {
	t.Helper()
	if cmd == nil {
		t.Fatal("tick must at least re-arm the ticker")
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		batch, ok := msg.(tea.BatchMsg)
		if !ok {
			t.Fatalf("expected tea.BatchMsg, got %T", msg)
		}
		return len(batch)
	case <-time.After(500 * time.Millisecond):
		return 1
	}
}

// TestPodsMsgFromStaleNamespaceDropped is the namespace-fencing regression:
// an answer that raced a namespace switch within one generation must not
// mark the view loaded nor touch the raw cache — but it still releases the
// busy flag so the next tick can fetch again.
func TestPodsMsgFromStaleNamespaceDropped(t *testing.T) {
	t.Parallel()

	m := New(nil, "default")
	m.busy = true

	upd, _ := m.Update(podsMsg{
		gen:       m.gen,
		namespace: "other",
		pods:      []k8s.Pod{{Name: "p1", Namespace: "other"}},
	})
	m = upd.(Model)

	if m.loaded {
		t.Error("stale-namespace podsMsg must not mark the view loaded")
	}
	if len(m.raw) != 0 {
		t.Errorf("stale-namespace podsMsg must not touch the raw cache, got %d pods", len(m.raw))
	}
	if m.busy {
		t.Error("stale-namespace podsMsg must still release the busy flag")
	}
}

// TestPodsMsgAppliesAndClearsBusy covers the happy path: matching namespace,
// data lands in the table and the busy flag is released.
func TestPodsMsgAppliesAndClearsBusy(t *testing.T) {
	t.Parallel()

	m := New(nil, "default")
	m.busy = true

	upd, _ := m.Update(podsMsg{
		gen:       m.gen,
		namespace: "default",
		pods:      []k8s.Pod{{Name: "p1", Namespace: "default"}},
	})
	m = upd.(Model)

	if m.busy {
		t.Error("podsMsg must release the busy flag")
	}
	if !m.loaded {
		t.Error("podsMsg must mark the view loaded")
	}
	if rows := len(m.table.Rows()); rows != 1 {
		t.Errorf("table rows = %d, want 1", rows)
	}
}

// TestPodsMsgErrorClearsBusy pins the "no stuck view" rule: a failed fetch
// releases busy too, otherwise one API error would freeze the view forever.
func TestPodsMsgErrorClearsBusy(t *testing.T) {
	t.Parallel()

	m := New(nil, "default")
	m.busy = true

	upd, _ := m.Update(podsMsg{gen: m.gen, namespace: "default", err: errors.New("boom")})
	m = upd.(Model)

	if m.busy {
		t.Error("failed podsMsg must release the busy flag")
	}
	if !m.loaded {
		t.Error("failed podsMsg must still mark the view loaded")
	}
	if m.err == nil {
		t.Error("failed podsMsg must surface the error")
	}
}

// TestTickSkipsFetchWhileBusy drives the tick handler twice: the first tick
// spawns ticker + fetch and raises busy, the second must re-arm only the
// ticker while the first fetch is still in flight.
func TestTickSkipsFetchWhileBusy(t *testing.T) {
	t.Parallel()

	m := New(&k8s.Client{}, "default")

	upd, cmd := m.Update(tickMsg{gen: m.gen})
	m = upd.(Model)
	if !m.busy {
		t.Fatal("first tick should raise the busy flag")
	}
	if n := tickBatchSize(t, cmd); n != 2 {
		t.Errorf("first tick batch size = %d, want 2 (ticker + fetch)", n)
	}

	upd, cmd = m.Update(tickMsg{gen: m.gen})
	m = upd.(Model)
	if n := tickBatchSize(t, cmd); n != 1 {
		t.Errorf("busy tick batch size = %d, want 1 (ticker only)", n)
	}
}

// TestTickAfterBusyReleaseFetchesAgain completes the cycle: once the data
// message releases busy, the next tick fetches again.
func TestTickAfterBusyReleaseFetchesAgain(t *testing.T) {
	t.Parallel()

	m := New(&k8s.Client{}, "default")

	upd, _ := m.Update(tickMsg{gen: m.gen})
	m = upd.(Model)
	upd, _ = m.Update(podsMsg{gen: m.gen, namespace: "default", err: errors.New("boom")})
	m = upd.(Model)

	upd, cmd := m.Update(tickMsg{gen: m.gen})
	m = upd.(Model)
	if n := tickBatchSize(t, cmd); n != 2 {
		t.Errorf("post-release tick batch size = %d, want 2 (ticker + fetch)", n)
	}
}
