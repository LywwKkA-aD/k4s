// Package services is the services list view: kubectl-style columns (TYPE,
// CLUSTER-IP, EXTERNAL-IP, PORT(S)), Enter to describe, 'l' to tail the
// logs of the pods behind a service and 'f' to port-forward it.
//
// The list refreshes through an async.Poller; row actions resolve through
// async.Run under an InFlight guard. The view keeps a views.State, so Esc
// from describe or logs brings it back as it was left.
package services

import (
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
	minTableRows = 5

	// A list of every namespace can be large; it gets more time than one
	// namespace before the poller counts it as slow.
	namespaceTimeout     = 5 * time.Second
	allNamespacesTimeout = 15 * time.Second
)

// Model is the services list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	clock     async.Clock

	table  table.Model
	raw    []k8s.Service
	err    error
	loaded bool
	bodyH  int
	filter filter.Model

	// tok is the view's mint: the poller runs under its first Token, each
	// row action under a Next one.
	tok  async.Token
	poll async.Poller
	acts async.InFlight
	// logsFor is the row the logs request in flight was made for.
	logsFor views.ResourceRef
	// pendingKey is the row a restored State selected, until the first
	// list arrives to put the cursor on it; pendingCursor is where the
	// cursor goes if that row is gone.
	pendingKey    string
	pendingCursor int

	selectKey  key.Binding
	logsKey    key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
	filterKey  key.Binding
	forwardKey key.Binding
}

// Option changes how New builds the view.
type Option func(Model) Model

// WithClock arms the refresh ticks and the slow-logs notice on c instead
// of the real clock, so tests drive time by hand.
func WithClock(c async.Clock) Option {
	return func(m Model) Model {
		m.clock = c
		return m
	}
}

// New constructs a services view. With a client the first load is under
// way, and Init sends it.
func New(client *k8s.Client, namespace string, opts ...Option) Model {
	m := Model{
		client:     client,
		namespace:  namespace,
		clock:      async.RealClock{},
		table:      newTable(namespace == ""),
		filter:     filter.New(),
		tok:        async.New(),
		selectKey:  key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe")),
		logsKey:    key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "tail all pods")),
		forwardKey: key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "port-forward")),
		filterKey:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		// Help labels w with the poller's cadence.
		watchKey:   key.NewBinding(key.WithKeys("w")),
		refreshKey: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	for _, opt := range opts {
		m = opt(m)
	}
	m.poll = async.NewPoller(m.tok, async.Config{
		Enabled: client != nil,
		Timeout: listTimeout(namespace),
		Clock:   m.clock,
	})
	if client != nil {
		m.poll, _ = m.poll.Begin()
	}
	return m
}

func listTimeout(namespace string) time.Duration {
	if namespace == "" {
		return allNamespacesTimeout
	}
	return namespaceTimeout
}

func newTable(showNamespace bool) table.Model {
	t := table.New(
		table.WithColumns(tableColumns(showNamespace)),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)
	return t
}

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 7)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	return append(cols,
		table.Column{Title: "NAME", Width: 28},
		table.Column{Title: "TYPE", Width: 14},
		table.Column{Title: "CLUSTER-IP", Width: 16},
		table.Column{Title: "EXTERNAL-IP", Width: 18},
		table.Column{Title: "PORT(S)", Width: 22},
		table.Column{Title: "AGE", Width: 8},
	)
}

// Init sends the first load; a view without a client has none.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.list(pollRead)
}

// Title implements views.View. Stable routing name.
func (m Model) Title() string { return "services" }

// Command implements views.View. k4s polls rather than holding a watch
// open, so the command has no --watch.
func (m Model) Command() kubectl.Cmd {
	if m.namespace == "" {
		return kubectl.New("get", "services", "-A")
	}
	return kubectl.New("get", "services", "-n", m.namespace)
}

// Help implements views.View. The w binding is labelled with the refresh
// cadence: the footer has no --watch, so this is where watch state shows.
func (m Model) Help() []key.Binding {
	watch := m.watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{m.selectKey, m.logsKey, m.forwardKey, m.filterKey, watch, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. The view holds no stream or goroutine; a
// reply still in flight is dropped by its Token.
func (m Model) Close() error { return nil }
