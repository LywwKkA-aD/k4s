// Package namespaces is the namespace list view; selecting a row emits a
// NamespaceSelectedMsg the root model handles to switch the active namespace.
package namespaces

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// listTimeout bounds one list. Namespaces are few, so the 15s budget
	// of an all-namespaces list does not apply.
	listTimeout  = 5 * time.Second
	minTableRows = 5
	allRowName   = "<all>"
)

// Model is the namespaces list view.
type Model struct {
	client *k8s.Client
	// poll owns the refresh cadence and fences replies: a list answered
	// for another instance of this view carries another Token.
	poll   async.Poller
	table  table.Model
	raw    []k8s.Namespace
	err    error
	loaded bool
	bodyH  int
	filter filter.Model
	// pending is the row a Restore asked for, held until the first list.
	pending pendingRow

	selectKey  key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
	filterKey  key.Binding
}

// Option configures New.
type Option func(options) options

type options struct {
	clock async.Clock
}

// WithClock arms the refresh ticks on clock instead of real timers; tests
// pass an asynctest.Clock and fire the ticks themselves.
func WithClock(clock async.Clock) Option {
	return func(o options) options {
		o.clock = clock
		return o
	}
}

// New constructs the namespaces view. With a client its first list is
// already under way: Init sends it.
func New(client *k8s.Client, opts ...Option) Model {
	var o options
	for _, opt := range opts {
		o = opt(o)
	}
	poll := async.NewPoller(async.New(), async.Config{
		Timeout: listTimeout,
		Enabled: client != nil,
		Clock:   o.clock,
	})
	if client != nil {
		poll, _ = poll.Begin()
	}

	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "NAME", Width: 32},
			{Title: "STATUS", Width: 12},
			{Title: "AGE", Width: 18},
		}),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)

	return Model{
		client:     client,
		poll:       poll,
		table:      t,
		filter:     filter.New(),
		selectKey:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select")),
		watchKey:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		refreshKey: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
		filterKey:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	}
}

// Init sends the first list, as a refresh tick would.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.list(k8s.FromCache())
}

// list asks for the namespaces under the poller's current Token.
func (m Model) list(opts ...k8s.ListOption) tea.Cmd {
	c := m.client
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) ([]k8s.Namespace, error) {
		return c.ListNamespaces(ctx, opts...)
	})
}

// Update handles size, refresh, selection and forwards keystrokes to the table.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// msg.Height is already the body height (root model adjusts it).
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
	case async.TickMsg:
		var due bool
		if m.poll, due = m.poll.Due(msg); due {
			return m, m.list(k8s.FromCache())
		}
		return m, nil
	case async.Result[[]k8s.Namespace]:
		return m.onList(msg)
	case tea.PasteMsg:
		next, cmd, _ := m.feedFilter(msg)
		return next, cmd
	case tea.KeyPressMsg:
		if next, cmd, consumed := m.feedFilter(msg); consumed {
			return next, cmd
		}
		if next, cmd, handled := m.handleKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// onList takes a list reply meant for this view. Done arms the next tick,
// so its Cmd must reach Bubble Tea even when the list failed.
func (m Model) onList(msg async.Result[[]k8s.Namespace]) (views.View, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil
	}
	var next tea.Cmd
	m.poll, next = m.poll.Done(msg.Token, msg.Err)
	m.loaded = true
	m.err = msg.Err
	if msg.Err != nil {
		return m, next
	}
	m.raw = msg.Value
	m, gone := m.withRows().landPending()
	return m, tea.Batch(next, gone)
}

// handleKey reports handled when the key matched a binding.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Open()
		return m.withRows(), cmd, true
	case key.Matches(msg, m.watchKey):
		var cmd tea.Cmd
		m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled() && m.client != nil)
		return m, cmd, true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		var ok bool
		if m.poll, ok = m.poll.Begin(); !ok {
			return m, nil, true
		}
		return m, m.list(), true
	case key.Matches(msg, m.selectKey) && m.loaded:
		row := m.table.SelectedRow()
		if row == nil {
			return m, nil, false
		}
		name := row[0]
		if name == allRowName {
			name = ""
		}
		return m, func() tea.Msg { return views.NamespaceSelectedMsg{Namespace: name} }, true
	}
	return m, nil, false
}

// Title implements views.View. Stable routing name.
func (m Model) Title() string { return "namespaces" }

// Command implements views.View. k4s polls rather than holding a watch
// open, so the command has no --watch.
func (m Model) Command() kubectl.Cmd {
	return kubectl.New("get", "namespaces")
}

// Help implements views.View. The w hint is the refresh cadence.
func (m Model) Help() []key.Binding {
	watch := m.watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{m.selectKey, m.filterKey, watch, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. No streaming resources held.
func (m Model) Close() error { return nil }
