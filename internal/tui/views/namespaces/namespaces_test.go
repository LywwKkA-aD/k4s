package namespaces

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

// TestNamespacesMsgErrorClearsBusy pins the "no stuck view" rule: a failed
// fetch releases busy too, otherwise one API error would freeze the view.
func TestNamespacesMsgErrorClearsBusy(t *testing.T) {
	t.Parallel()

	m := New(nil)
	m.busy = true

	upd, _ := m.Update(namespacesMsg{gen: m.gen, err: errors.New("boom")})
	m = upd.(Model)

	if m.busy {
		t.Error("failed namespacesMsg must release the busy flag")
	}
	if !m.loaded {
		t.Error("failed namespacesMsg must still mark the view loaded")
	}
	if m.err == nil {
		t.Error("failed namespacesMsg must surface the error")
	}
}

// TestNamespacesMsgAppliesAndClearsBusy covers the happy path.
func TestNamespacesMsgAppliesAndClearsBusy(t *testing.T) {
	t.Parallel()

	m := New(nil)
	m.busy = true

	upd, _ := m.Update(namespacesMsg{
		gen:   m.gen,
		items: []k8s.Namespace{{Name: "default"}},
	})
	m = upd.(Model)

	if m.busy {
		t.Error("namespacesMsg must release the busy flag")
	}
	// The synthetic "<all>" row plus the one fetched namespace.
	if rows := len(m.table.Rows()); rows != 2 {
		t.Errorf("table rows = %d, want 2", rows)
	}
}

// TestTickSkipsFetchWhileBusy drives the tick handler twice: the first tick
// spawns ticker + fetch and raises busy, the second must re-arm only the
// ticker while the first fetch is still in flight.
func TestTickSkipsFetchWhileBusy(t *testing.T) {
	t.Parallel()

	m := New(&k8s.Client{})

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
