package deployments

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// The ResourceVersion a list asks for. A poll (the first load, a tick, a
// restore) takes the apiserver's watch cache, which on k3s spares kine a
// query; r is the user asking for the truth now, a consistent read.
const (
	watchCacheRV = "0"
	consistentRV = ""
)

func (m Model) list(resourceVersion string) tea.Cmd {
	c, ns := m.client, m.namespace
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) ([]k8s.Deployment, error) {
		return c.ListDeploymentsAt(ctx, ns, resourceVersion)
	})
}

func (m Model) onTick(msg async.TickMsg) (Model, tea.Cmd) {
	var due bool
	if m.poll, due = m.poll.Due(msg); !due {
		return m, nil
	}
	return m, m.list(watchCacheRV)
}

// onList folds a list reply in. The poller arms the next tick; an error
// keeps the old rows behind the error text.
func (m Model) onList(msg async.Result[[]k8s.Deployment]) (Model, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil
	}
	var arm tea.Cmd
	m.poll, arm = m.poll.Done(msg.Token, msg.Err)
	m.err, m.loaded = msg.Err, true
	if msg.Err != nil {
		return m, arm
	}
	m.raw = msg.Value
	m, gone := m.withRows().settleRestore()
	return m, tea.Batch(arm, gone)
}

// feedFilter hands a key press or a paste to the open filter and refilters
// the rows. It reports false when the filter is closed or let the key pass.
func (m Model) feedFilter(msg tea.Msg) (Model, tea.Cmd, bool) {
	if !m.filter.IsOpen() {
		return m, nil, false
	}
	var cmd tea.Cmd
	var consumed bool
	m.filter, cmd, consumed = m.filter.Update(msg)
	if consumed {
		m = m.withRows()
	}
	return m, cmd, consumed
}

// withRows narrows raw by the filter into the table. The cursor stays on
// its deployment while that row is shown; otherwise it keeps its index,
// clamped to the rows.
func (m Model) withRows() Model {
	at, had := m.cursorKey()
	shown := m.visible()
	m.shown = shown
	m.table.SetRows(toRows(shown, m.namespace == ""))
	cursor := max(m.table.Cursor(), 0)
	if i := indexOf(shown, at); had && i >= 0 {
		cursor = i
	}
	m.table.SetCursor(cursor)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	return m
}

func (m Model) visible() []k8s.Deployment {
	if !m.filter.Active() {
		return m.raw
	}
	out := make([]k8s.Deployment, 0, len(m.raw))
	for _, d := range m.raw {
		if m.filter.Match(d.Name, d.Namespace) {
			out = append(out, d)
		}
	}
	return out
}

func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
}

// selected is the deployment under the cursor.
func (m Model) selected() (k8s.Deployment, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.shown) {
		return k8s.Deployment{}, false
	}
	return m.shown[i], true
}

// cursorKey is the views.RowKey of the row under the cursor.
func (m Model) cursorKey() (string, bool) {
	d, ok := m.selected()
	if !ok {
		return "", false
	}
	return views.RowKey(d.Namespace, d.Name), true
}

func indexOf(items []k8s.Deployment, rowKey string) int {
	return slices.IndexFunc(items, func(d k8s.Deployment) bool {
		return views.RowKey(d.Namespace, d.Name) == rowKey
	})
}

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 6)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	return append(cols,
		table.Column{Title: "NAME", Width: 30},
		table.Column{Title: "READY", Width: 8},
		table.Column{Title: "UP-TO-DATE", Width: 11},
		table.Column{Title: "AVAILABLE", Width: 10},
		table.Column{Title: "AGE", Width: 8},
	)
}

func toRows(items []k8s.Deployment, showNamespace bool) []table.Row {
	rows := make([]table.Row, 0, len(items))
	for _, d := range items {
		row := make(table.Row, 0, 6)
		if showNamespace {
			row = append(row, d.Namespace)
		}
		rows = append(rows, append(row,
			d.Name,
			fmt.Sprintf("%d/%d", d.Ready, d.Replicas),
			strconv.Itoa(int(d.UpToDate)),
			strconv.Itoa(int(d.Available)),
			k8s.HumanizeDuration(d.Age),
		))
	}
	return rows
}
