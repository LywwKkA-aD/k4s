package namespaces

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pendingRow is the row a restored view puts its cursor on once its first
// list arrives; set is false when nothing waits.
type pendingRow struct {
	set    bool
	key    string
	cursor int
}

// State implements views.Stateful: the committed filter, the row under the
// cursor and whether watch is off. Until a restored view has its first
// list, the row it is waiting for stands in for the empty table's.
func (m Model) State() views.State {
	st := views.State{Paused: !m.poll.Enabled()}
	if !m.filter.IsOpen() {
		st.Filter = m.filter.Query()
	}
	if m.pending.set {
		st.Selected, st.Cursor = m.pending.key, m.pending.cursor
		return st
	}
	if name, ok := m.cursorName(); ok {
		st.Selected = views.RowKey("", name)
	}
	st.Cursor = max(m.table.Cursor(), 0)
	return st
}

// Restore implements views.Restorer. It starts no list of its own: the
// root runs Init next, which sends the first one as a tick would.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	m.filter = m.filter.WithQuery(st.Filter)
	m = m.withRows()
	m.pending = pendingRow{set: true, key: st.Selected, cursor: st.Cursor}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithEnabled(!st.Paused && m.client != nil)
	if !m.loaded {
		return m, cmd
	}
	m, gone := m.landPending()
	return m, tea.Batch(cmd, gone)
}

// landPending puts the cursor on the pending row. A row that has gone
// leaves the cursor at the saved index, clamped, and says so.
func (m Model) landPending() (Model, tea.Cmd) {
	if !m.pending.set {
		return m, nil
	}
	p := m.pending
	m.pending = pendingRow{}
	rows := len(m.table.Rows())
	for i := range rows {
		if name, _ := m.rowName(i); views.RowKey("", name) == p.key {
			return m.withCursor(i), nil
		}
	}
	m = m.withCursor(min(p.cursor, rows-1))
	if p.key == "" {
		return m, nil
	}
	text := "namespace " + p.key + " is gone"
	return m, func() tea.Msg { return views.StatusMsg{Level: status.Info, Text: text} }
}

// withCursor moves the cursor to row i through the table's own scrolling,
// which keeps the row in view; SetCursor alone can leave it just below.
func (m Model) withCursor(i int) Model {
	m.table.GotoTop()
	m.table.MoveDown(max(i, 0))
	return m
}

// Back implements views.BackHandler: Esc first clears a committed filter.
// An open filter never gets here; it has the keyboard and closes itself.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	if m.filter.IsOpen() || m.filter.Query() == "" {
		return m, nil, false
	}
	m.filter = m.filter.Close()
	return m.withRows(), nil, true
}
