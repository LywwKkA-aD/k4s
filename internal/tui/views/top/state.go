package top

import (
	"strings"

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

// State implements views.Stateful: mode, sort, the row under the cursor
// and whether watch is off. Until a restored view has its first list, the
// row it is waiting for stands in for the empty table's.
func (m Model) State() views.State {
	st := views.State{Paused: !m.poll.Enabled(), Mode: string(m.mode), Sort: string(m.sort)}
	if m.pending.set {
		st.Selected, st.Cursor = m.pending.key, m.pending.cursor
		return st
	}
	if c := m.table.Cursor(); c >= 0 && c < len(m.keys) {
		st.Selected = m.keys[c]
	}
	st.Cursor = max(m.table.Cursor(), 0)
	return st
}

// Restore implements views.Restorer. It sends nothing: the root runs Init
// next, which fetches the restored mode as a tick would. A mode other than
// the one New began moves the poller to a fresh Token, so nothing asked
// for the old mode can land.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	if md := modeOf(st.Mode); md != m.mode {
		m = m.withMode(md)
		if m.client != nil {
			m.mint = m.mint.Next()
			m.poll = m.poll.Restart(m.mint)
		}
	}
	m = m.withSort(sortOf(st.Sort)).withRows()
	m.pending = pendingRow{set: true, key: st.Selected, cursor: st.Cursor}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithEnabled(!st.Paused && m.client != nil)
	return m, cmd
}

func modeOf(s string) mode {
	if mode(s) == modeNodes {
		return modeNodes
	}
	return modePods
}

func sortOf(s string) sortKey {
	if sortKey(s) == sortMemory {
		return sortMemory
	}
	return sortCPU
}

// landPending puts the cursor on the pending row. A row that has gone
// leaves the cursor at the saved index, clamped, and says so.
func (m Model) landPending() (Model, tea.Cmd) {
	if !m.pending.set {
		return m, nil
	}
	p := m.pending
	m.pending = pendingRow{}
	for i, k := range m.keys {
		if k == p.key {
			return m.withCursor(i), nil
		}
	}
	m = m.withCursor(min(p.cursor, len(m.keys)-1))
	if p.key == "" {
		return m, nil
	}
	_, name, _ := strings.Cut(p.key, "/")
	if name == "" {
		name = p.key
	}
	text := strings.TrimSuffix(string(m.mode), "s") + " " + name + " is gone"
	return m, func() tea.Msg { return views.StatusMsg{Level: status.Info, Text: text} }
}

// withCursor moves the cursor to row i through the table's own scrolling,
// which keeps the row in view; SetCursor alone can leave it just below.
func (m Model) withCursor(i int) Model {
	m.table.GotoTop()
	m.table.MoveDown(max(i, 0))
	return m
}
