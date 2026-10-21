package services

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// State implements views.Stateful. Until the first list after a Restore
// it still reports the row that Restore asked for.
func (m Model) State() views.State {
	st := views.State{
		Filter:   m.committedFilter(),
		Selected: m.selectedKey(),
		Cursor:   max(m.table.Cursor(), 0),
		Paused:   m.client != nil && !m.poll.Enabled(),
	}
	if m.pendingKey != "" {
		st.Selected, st.Cursor = m.pendingKey, m.pendingCursor
	}
	return st
}

// committedFilter is the query of a closed filter; one still being typed
// is not part of the State.
func (m Model) committedFilter() string {
	if m.filter.IsOpen() {
		return ""
	}
	return m.filter.Query()
}

// Restore implements views.Restorer. It starts no fetch: the root runs
// Init next, and that first load reads the watch cache as a tick does.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	m.filter = m.filter.WithQuery(st.Filter)
	m.applyFilter("")
	var cmd tea.Cmd
	if st.Paused {
		m.poll, cmd = m.poll.WithEnabled(false)
	}
	m.pendingKey, m.pendingCursor = st.Selected, st.Cursor
	return m, cmd
}

// settlePending puts the cursor on the row a restored State selected, once
// the first list is in. A row that is gone leaves the cursor at the
// State's index, clamped to the list, and the status line says so.
func (m Model) settlePending() (Model, tea.Cmd) {
	key, cursor := m.pendingKey, m.pendingCursor
	if key == "" {
		return m, nil
	}
	m.pendingKey, m.pendingCursor = "", 0
	if i := m.rowIndex(key); i >= 0 {
		m.moveCursor(i)
		return m, nil
	}
	if n := len(m.table.Rows()); n > 0 {
		m.moveCursor(min(max(cursor, 0), n-1))
	}
	return m, statusCmd(status.Info, "service "+m.rowName(key)+" is gone")
}

// rowName is a row key as the user knows it: the name alone in a
// namespaced view, namespace/name across all namespaces.
func (m Model) rowName(key string) string {
	if m.namespace == "" {
		return key
	}
	return strings.TrimPrefix(key, m.namespace+"/")
}

// Back implements views.BackHandler: Esc clears a committed filter and
// stays, with the cursor on the row it was on.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	if m.filter.IsOpen() || m.filter.Query() == "" {
		return m, nil, false
	}
	keep := m.selectedKey()
	m.filter = m.filter.Close()
	m.applyFilter(keep)
	return m, nil, true
}
