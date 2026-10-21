package pods

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pendingRow is the row a restored State had under the cursor. The rows
// arrive with the first list, so the cursor is placed then.
type pendingRow struct {
	key    string
	cursor int
	set    bool
}

// State implements views.Stateful: the committed filter (not one being
// typed), the row under the cursor and whether watch is off.
func (m Model) State() views.State {
	st := views.State{Paused: m.client != nil && !m.poll.Enabled()}
	if !m.filter.IsOpen() {
		st.Filter = m.filter.Query()
	}
	if m.pending.set {
		st.Selected, st.Cursor = m.pending.key, m.pending.cursor
		return st
	}
	if pod, ok := m.cursorPod(); ok {
		st.Selected, st.Cursor = rowKey(pod), m.table.Cursor()
	}
	return st
}

// Restore implements views.Restorer on a view fresh from New. The filter
// applies at once; the row waits for the list Init sends, which reads the
// watch cache like any tick. Restore itself starts no list.
func (m Model) Restore(st views.State) (views.View, tea.Cmd) {
	m.filter = m.filter.WithQuery(st.Filter)
	m = m.refiltered()
	m.pending = pendingRow{key: st.Selected, cursor: st.Cursor, set: true}
	var watch tea.Cmd
	m.poll, watch = m.poll.WithEnabled(!st.Paused && m.client != nil)
	if !m.loaded {
		return m, watch
	}
	m, gone := m.settlePending()
	return m, tea.Batch(watch, gone)
}

// settlePending puts the cursor on the restored row, or, when that pod is
// gone, on the row at its old index and says so.
func (m Model) settlePending() (Model, tea.Cmd) {
	p := m.pending
	if !p.set {
		return m, nil
	}
	m.pending = pendingRow{}
	if i := indexOf(m.shown, p.key); i >= 0 {
		return m.withCursor(i), nil
	}
	m = m.withCursor(p.cursor)
	if p.key == "" {
		return m, nil
	}
	return m, statusCmd(status.Info, "pod "+nameOf(p.key)+" is gone")
}

// Back implements views.BackHandler: Esc clears a committed filter before
// it leaves the view. The cursor stays on its pod.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	if m.filter.IsOpen() || m.filter.Query() == "" {
		return m, nil, false
	}
	m.filter = m.filter.Close()
	return m.refiltered(), nil, true
}

// nameOf is the pod name in a RowKey.
func nameOf(key string) string {
	_, name, found := strings.Cut(key, "/")
	if !found {
		return key
	}
	return name
}
