package services

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Resource versions of a services list (M4.4): a poll may be answered
// from the apiserver's watch cache, a refresh the user asked for reads
// through.
const (
	pollRead       = "0"
	consistentRead = ""
)

// Update routes ticks, replies and keys; navigation goes to the table.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
		return m, nil
	case async.TickMsg:
		return m.onTick(msg)
	case async.Result[[]k8s.Service]:
		return m.onList(msg)
	case async.Result[k8s.LogTarget]:
		return m.onLogTarget(msg)
	case logsSlowMsg:
		return m, m.onLogsSlow(msg)
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

// list fetches the services under the poller's current Token.
func (m Model) list(resourceVersion string) tea.Cmd {
	c, ns := m.client, m.namespace
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) ([]k8s.Service, error) {
		return c.ListServices(ctx, ns, resourceVersion)
	})
}

func (m Model) onTick(msg async.TickMsg) (Model, tea.Cmd) {
	var ok bool
	if m.poll, ok = m.poll.Due(msg); !ok {
		return m, nil
	}
	return m, m.list(pollRead)
}

// onList folds a list reply in and arms the next tick. A failed refresh
// keeps the rows it had, but the view shows the error.
func (m Model) onList(msg async.Result[[]k8s.Service]) (Model, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil
	}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.Done(msg.Token, msg.Err)
	m.err = msg.Err
	m.loaded = true
	if msg.Err != nil {
		return m, cmd
	}
	m.raw = msg.Value
	m.applyFilter(m.selectedKey())
	return m, cmd
}

func (m Model) onKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		next, cmd := m.openFilter()
		return next, cmd, true
	case key.Matches(msg, m.watchKey):
		var cmd tea.Cmd
		m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled() && m.client != nil)
		return m, cmd, true
	case key.Matches(msg, m.refreshKey):
		next, cmd := m.refresh()
		return next, cmd, true
	case key.Matches(msg, m.selectKey):
		return m, m.describe(), true
	case key.Matches(msg, m.logsKey):
		next, cmd := m.logs()
		return next, cmd, true
	case key.Matches(msg, m.forwardKey):
		return m, m.forward(), true
	}
	return m, nil, false
}

// refresh is 'r': a consistent read now, unless a list is in flight.
func (m Model) refresh() (Model, tea.Cmd) {
	if m.client == nil {
		return m, statusCmd(status.Warn, "no kubeconfig")
	}
	var ok bool
	if m.poll, ok = m.poll.Begin(); !ok {
		return m, nil
	}
	return m, m.list(consistentRead)
}

func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
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
