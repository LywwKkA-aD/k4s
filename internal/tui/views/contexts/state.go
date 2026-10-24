package contexts

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pendingRow is the row a restored picker puts its cursor on once the
// kubeconfig has been read; set is false when nothing waits.
type pendingRow struct {
	set    bool
	key    string
	cursor int
}

// State implements views.Stateful: the context under the cursor. The
// picker has no filter and no refresh cadence to keep. Until a restored
// picker has read the kubeconfig, the row it waits for stands in for the
// empty table's.
func (m Model) State() views.State {
	if m.pending.set {
		return views.State{Selected: m.pending.key, Cursor: m.pending.cursor}
	}
	st := views.State{Cursor: max(m.table.Cursor(), 0)}
	if name, ok := m.cursorName(); ok {
		st.Selected = views.RowKey("", name)
	}
	return st
}

// Restore implements views.Restorer. It reads nothing itself: the root
// runs Init next, which reads the kubeconfig.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	m.pending = pendingRow{set: true, key: st.Selected, cursor: st.Cursor}
	if !m.loaded {
		return m, nil
	}
	return m.landPending()
}

// landPending puts the cursor on the pending row. A context that has gone
// from the kubeconfig leaves the cursor at the saved index, clamped, and
// says so.
func (m Model) landPending() (Model, tea.Cmd) {
	if !m.pending.set {
		return m, nil
	}
	p := m.pending
	m.pending = pendingRow{}
	for i, name := range m.names {
		if views.RowKey("", name) == p.key {
			return m.withCursor(i), nil
		}
	}
	m = m.withCursor(min(p.cursor, len(m.names)-1))
	if p.key == "" {
		return m, nil
	}
	text := "context " + p.key + " is gone"
	return m, func() tea.Msg { return views.StatusMsg{Level: status.Info, Text: text} }
}

// withCursor moves the cursor to row i through the table's own scrolling,
// which keeps the row in view; SetCursor alone can leave it just below.
func (m Model) withCursor(i int) Model {
	m.table.GotoTop()
	m.table.MoveDown(max(i, 0))
	return m
}
