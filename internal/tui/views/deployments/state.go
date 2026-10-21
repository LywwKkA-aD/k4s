package deployments

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pendingRow is the row a restored view puts its cursor on once the first
// list arrives: rows are not kept in history, so until then there is no
// row to point at.
type pendingRow struct {
	key    string
	cursor int
	set    bool
}

// State implements views.Stateful: the committed filter, the row under
// the cursor and whether watch is on. A filter still being typed is not
// kept.
func (m Model) State() views.State {
	st := views.State{Paused: !m.watch}
	if !m.filter.IsOpen() {
		st.Filter = m.filter.Query()
	}
	if m.pending.set {
		st.Selected, st.Cursor = m.pending.key, m.pending.cursor
		return st
	}
	if at, ok := m.cursorKey(); ok {
		st.Selected, st.Cursor = at, m.table.Cursor()
	}
	return st
}

// Restore implements views.Restorer. It starts no list of its own: the
// root runs Init next, which lists from the watch cache like a tick.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	m.filter = m.filter.WithQuery(st.Filter)
	m.watch = !st.Paused
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithEnabled(m.watch && m.client != nil)
	if st.Selected != "" {
		m.pending = pendingRow{key: st.Selected, cursor: st.Cursor, set: true}
	}
	return m.withRows(), cmd
}

// settleRestore puts the cursor on the restored row once rows are in. A
// row gone meanwhile leaves the cursor at the old index, clamped, and says
// so.
func (m Model) settleRestore() (Model, tea.Cmd) {
	p := m.pending
	if !p.set {
		return m, nil
	}
	m.pending = pendingRow{}
	if i := indexOf(m.shown, p.key); i >= 0 {
		m.table.SetCursor(i)
		return m, nil
	}
	m.table.SetCursor(p.cursor)
	return m, statusCmd(status.Info, kind+" "+nameOf(p.key)+" is gone")
}

// Back implements views.BackHandler: Esc clears a committed filter before
// it leaves the view, and the cursor stays on its row.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	if m.filter.IsOpen() || m.filter.Query() == "" {
		return m, nil, false
	}
	m.filter = m.filter.Close()
	return m.withRows(), nil, true
}

// nameOf is the name part of a views.RowKey.
func nameOf(rowKey string) string {
	return rowKey[strings.LastIndex(rowKey, "/")+1:]
}
