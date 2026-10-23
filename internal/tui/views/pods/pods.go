// Package pods is the pod list view: `kubectl get pods`, with describe,
// logs, exec and port-forward on the row under the cursor.
package pods

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

var (
	describeKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe"))
	logsKey     = key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "logs"))
	execKey     = key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "exec"))
	forwardKey  = key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "port-forward"))
	filterKey   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	refreshKey  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
	watchKey    = key.NewBinding(key.WithKeys("w"))
)

// Model is the pods list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	clock     async.Clock

	table  table.Model
	filter filter.Model
	bodyH  int
	width  int

	raw []k8s.Pod
	// shown is raw through the filter, in table order: the cursor indexes it.
	shown []k8s.Pod
	// health is the last list answer: an error over earlier rows is a stale
	// view, one before any rows is the body.
	health stale.Health
	loaded bool

	// tok is the view's mint: every row action takes the next Token.
	tok   async.Token
	poll  async.Poller
	acts  async.InFlight
	asked asked

	// pending is a restored State's row, held until the first list lands.
	pending pendingRow
}

// Option changes how New builds the view.
type Option func(Model) Model

// WithClock arms the refresh ticks and the "resolving" notice on clk
// instead of the real clock, so tests step time by hand.
func WithClock(clk async.Clock) Option {
	return func(m Model) Model {
		m.clock = clk
		return m
	}
}

// New constructs a pods view for the given namespace ("" = all
// namespaces). With a client the first list is under way: Init sends it.
func New(client *k8s.Client, namespace string, opts ...Option) Model {
	m := Model{
		client:    client,
		namespace: namespace,
		clock:     async.RealClock{},
		filter:    filter.New(),
		tok:       async.New(),
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
	m.table = newTable(layout(0, namespace == "").columns())
	return m
}

// Init sends the first list, read like any poll from the watch cache.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.list(k8s.FromCache())
}

// Update routes list replies, ticks, row-action replies and keys.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg), nil
	case async.TickMsg:
		return m.onTick(msg)
	case async.Result[[]k8s.Pod]:
		return m.onList(msg)
	case async.Result[k8s.Containers]:
		return m.onResolved(msg)
	case resolvingMsg:
		return m, m.onResolving(msg)
	case tea.PasteMsg:
		return m.feedFilter(msg)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) onKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.filter.IsOpen() {
		return m.feedFilter(msg)
	}
	switch {
	case key.Matches(msg, filterKey):
		return m.openFilter()
	case key.Matches(msg, refreshKey):
		return m.refresh()
	case key.Matches(msg, watchKey):
		var cmd tea.Cmd
		m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled() && m.client != nil)
		return m, cmd
	case key.Matches(msg, describeKey):
		return m, m.describe()
	case key.Matches(msg, logsKey):
		return m.startAction(actLogs)
	case key.Matches(msg, execKey):
		return m.startAction(actExec)
	case key.Matches(msg, forwardKey):
		return m.startAction(actForward)
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// Title implements views.View. Routing depends on Title returning the
// stable view name.
func (m Model) Title() string { return "pods" }

// Command implements views.View. k4s polls rather than holding a watch
// open, so the command has no --watch.
func (m Model) Command() kubectl.Cmd {
	if m.namespace == "" {
		return kubectl.New("get", "pods", "-A")
	}
	return kubectl.New("get", "pods", "-n", m.namespace)
}

// Help implements views.View. The w binding names the refresh cadence,
// since the footer no longer marks watch state anywhere else.
func (m Model) Help() []key.Binding {
	watch := watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{describeKey, logsKey, execKey, forwardKey, filterKey, watch, refreshKey}
}

// CapturesKeys implements views.View. Returns true while the filter prompt
// is focused so that 'q' / ':' / '?' / etc. reach the input as text.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. Nothing outlives the view: a reply or tick
// still on its way is dropped by its Token wherever it lands.
func (m Model) Close() error { return nil }
