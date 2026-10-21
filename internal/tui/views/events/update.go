package events

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Update routes ticks, replies and keys; navigation goes to the table.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg), nil
	case async.TickMsg:
		return m.onTick(msg)
	case async.Result[k8s.EventList]:
		return m.onList(msg)
	case tea.PasteMsg:
		cmd, _ := m.feedFilter(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if cmd, consumed := m.feedFilter(msg); consumed {
			return m, cmd
		}
		if next, cmd, handled := m.onKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) resized(msg tea.WindowSizeMsg) Model {
	m.width, m.bodyH = msg.Width, msg.Height
	m.filter.SetWidth(msg.Width)
	m.table.SetColumns(layout(msg.Width, m.namespace == ""))
	styles.FitTable(&m.table)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	return m
}

// list fetches the events under the poller's current Token. A poll
// passes k8s.FromCache; a refresh the user asked for reads through.
func (m Model) list(opts ...k8s.ListOption) tea.Cmd {
	c, ns := m.client, m.namespace
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) (k8s.EventList, error) {
		return c.ListEvents(ctx, ns, opts...)
	})
}

func (m Model) onTick(msg async.TickMsg) (Model, tea.Cmd) {
	var ok bool
	if m.poll, ok = m.poll.Due(msg); !ok {
		return m, nil
	}
	return m, m.list(k8s.FromCache())
}

// onList folds a list reply in and arms the next tick. A failed refresh
// keeps the rows it had under a stale banner and says the error once; a
// forbidden -A moves to the context's namespace when there is one.
func (m Model) onList(msg async.Result[k8s.EventList]) (Model, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil
	}
	if m.canFallBack(msg.Err) {
		return m.fallBack()
	}
	var cmd, said tea.Cmd
	m.poll, cmd = m.poll.Done(msg.Token, msg.Err)
	m.health, said = m.health.Fold(explain(msg.Err, m.namespace))
	m.loaded = true
	if msg.Err != nil {
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
		return m, tea.Batch(cmd, said)
	}
	m.items = toItems(msg.Value.Items, m.namespace == "")
	m.cut = msg.Value.Cut
	m.applyFilter(m.selectedKey())
	m, gone := m.settlePending()
	return m, tea.Batch(cmd, said, gone)
}

// fallBack lists the context's namespace after -A was forbidden. The
// poller moves to a new epoch, so a late reply for -A is dropped.
func (m Model) fallBack() (Model, tea.Cmd) {
	ns := m.client.ContextNamespace
	m.namespace, m.fellBack = ns, true
	m.tok = m.tok.Next()
	m.poll = m.poll.Restart(m.tok)
	// The rows of an earlier -A list carry a NAMESPACE cell the new
	// columns do not have.
	m.items, m.shown, m.loaded, m.health = nil, nil, false, m.health.Reset()
	m.table.SetRows(nil)
	m = m.resized(tea.WindowSizeMsg{Width: m.width, Height: m.bodyH})
	said := statusCmd(status.Warn, "cluster-wide list forbidden; showing "+ns+" · "+canI(""))
	return m, tea.Batch(said, m.list(k8s.FromCache()), views.NamespaceFallback(ns))
}

func (m Model) onKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, filterKey):
		next, cmd := m.openFilter()
		return next, cmd, true
	case key.Matches(msg, warningsKey):
		return m.toggleWarnings(), nil, true
	case key.Matches(msg, watchKey):
		next, cmd := m.toggleWatch()
		return next, cmd, true
	case key.Matches(msg, refreshKey):
		next, cmd := m.refresh()
		return next, cmd, true
	case key.Matches(msg, describeKey):
		return m, m.describe(), true
	}
	return m, nil, false
}

// toggleWarnings is W: only the warnings, or every type again. The
// cursor stays on its event when that is still listed.
func (m Model) toggleWarnings() Model {
	m.warnings = !m.warnings
	m.applyFilter(m.selectedKey())
	return m
}

// toggleWatch is 'w': auto-refresh on or off. Without a client there is
// nothing to refresh.
func (m Model) toggleWatch() (Model, tea.Cmd) {
	if m.client == nil {
		return m, statusCmd(status.Warn, "no kubeconfig")
	}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled())
	return m, cmd
}

// refresh is 'r': a consistent read now, unless a list is in flight.
func (m Model) refresh() (Model, tea.Cmd) {
	if m.client == nil {
		return m, statusCmd(status.Warn, "no kubeconfig")
	}
	var ok bool
	if m.poll, ok = m.poll.Begin(); !ok {
		return m, statusCmd(status.Info, "refresh already running")
	}
	return m, m.list()
}

// tableHeight is the body less the lines above the table: the filter bar,
// the stale banner and the note.
func (m Model) tableHeight() int {
	h := m.bodyH
	if m.filter.Active() {
		h--
	}
	if m.health.Stale() {
		h--
	}
	if m.note() != "" {
		h--
	}
	return h
}

// feedFilter hands a key press or a paste to the open filter and refilters
// the rows. It reports false when the filter is closed or let the key pass.
func (m *Model) feedFilter(msg tea.Msg) (tea.Cmd, bool) {
	if !m.filter.IsOpen() {
		return nil, false
	}
	var cmd tea.Cmd
	var consumed bool
	m.filter, cmd, consumed = m.filter.Update(msg)
	if consumed {
		m.applyFilter("")
	}
	return cmd, consumed
}

func (m Model) openFilter() (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Open()
	m.applyFilter("")
	return m, cmd
}

func statusCmd(level status.Level, text string) tea.Cmd {
	return func() tea.Msg { return views.StatusMsg{Level: level, Text: text} }
}
