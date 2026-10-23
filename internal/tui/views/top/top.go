// Package top is the kubectl-top equivalent: per-pod or per-node CPU and
// memory pulled from the metrics-server REST endpoint and refreshed by an
// async.Poller like the other list views. 'n' switches between pod and
// node mode, 's' between sorting by CPU and by memory.
//
// When metrics-server is missing, unreachable or forbidden the view says
// so in the status row and the body, with a hint for the cluster's
// distribution (WithDistro) where a missing metrics-server is the cause,
// so the user can keep the rest of the TUI running.
package top

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	listTimeout = 5 * time.Second
	// allNamespacesTimeout bounds pods across all namespaces, whose answer
	// grows with the cluster.
	allNamespacesTimeout = 15 * time.Second
	minTableRows         = 5
)

// mode is what the view lists, as views.State.Mode names it.
type mode string

const (
	modePods  mode = "pods"
	modeNodes mode = views.ModeNodes
)

// sortKey is the column rows are sorted by, busiest first, as
// views.State.Sort names it.
type sortKey string

const (
	sortCPU    sortKey = "cpu"
	sortMemory sortKey = views.SortMemory
)

// Model is the top view.
type Model struct {
	client    *k8s.Client
	namespace string
	mode      mode
	sort      sortKey
	// distro words the hint for a missing metrics API.
	distro cluster.Distro

	// mint hands out the Token of each mode epoch; poll runs under the
	// current one, so a reply or tick from before an 'n' is dropped.
	mint async.Token
	poll async.Poller

	table table.Model
	pods  []k8s.PodMetric
	nodes []k8s.NodeMetric
	// keys holds views.RowKey of each table row, in row order.
	keys []string
	// health is the last answer: an error over earlier rows is a stale
	// view, one before any rows is the body.
	health stale.Health
	bodyH  int
	loaded bool
	// asked marks a fetch the user started (r, n): its failure is reported
	// even when it repeats the last one.
	asked bool
	// pending is the row a Restore asked for, held until the first list.
	pending pendingRow

	toggleKey  key.Binding
	sortKey    key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
}

// Option configures New.
type Option func(options) options

type options struct {
	clock  async.Clock
	distro cluster.Distro
}

// WithClock arms the refresh ticks on clock instead of real timers; tests
// pass an asynctest.Clock and fire the ticks themselves.
func WithClock(clock async.Clock) Option {
	return func(o options) options {
		o.clock = clock
		return o
	}
}

// WithDistro names the cluster's distribution, so a missing metrics API
// gets its hint (cluster.MetricsHint): k3s bundles metrics-server, so
// there it was switched off rather than never installed.
func WithDistro(d cluster.Distro) Option {
	return func(o options) options {
		o.distro = d
		return o
	}
}

// New constructs a top view scoped to the given namespace ("" = all). With
// a client its first fetch is already under way: Init sends it.
func New(client *k8s.Client, namespace string, opts ...Option) Model {
	var o options
	for _, opt := range opts {
		o = opt(o)
	}
	mint := async.New()
	poll := async.NewPoller(mint, async.Config{Enabled: client != nil, Clock: o.clock})
	if client != nil {
		poll, _ = poll.Begin()
	}
	return Model{
		client:     client,
		namespace:  namespace,
		mode:       modePods,
		sort:       sortCPU,
		distro:     o.distro,
		mint:       mint,
		poll:       poll,
		table:      newTable(modePods, namespace == ""),
		toggleKey:  key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "nodes/pods")),
		sortKey:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		watchKey:   key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		refreshKey: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}

// Init sends the first fetch for the current mode.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.fetch()
}

// fetch asks for the current mode's metrics under the poller's Token.
// Ticks and r send the same request: metrics-server answers from its
// latest scrape and has no watch cache, so ResourceVersion means nothing
// to it.
func (m Model) fetch() tea.Cmd {
	c, tok, timeout := m.client, m.poll.Token(), m.timeout()
	if m.mode == modeNodes {
		return async.Run(tok, timeout, c.ListNodeMetrics)
	}
	ns := m.namespace
	return async.Run(tok, timeout, func(ctx context.Context) ([]k8s.PodMetric, error) {
		return c.ListPodMetrics(ctx, ns)
	})
}

func (m Model) timeout() time.Duration {
	if m.mode == modePods && m.namespace == "" {
		return allNamespacesTimeout
	}
	return listTimeout
}

// Update handles size, ticks, key bindings and metrics replies.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
	case async.TickMsg:
		var due bool
		if m.poll, due = m.poll.Due(msg); due {
			return m, m.fetch()
		}
		return m, nil
	case tea.KeyPressMsg:
		if next, cmd, handled := m.handleKey(msg); handled {
			return next, cmd
		}
	case async.Result[[]k8s.PodMetric]:
		return m.onReply(msg.Token, msg.Err, func(m Model) Model { m.pods = msg.Value; return m })
	case async.Result[[]k8s.NodeMetric]:
		return m.onReply(msg.Token, msg.Err, func(m Model) Model { m.nodes = msg.Value; return m })
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// onReply takes a fetch reply meant for this view and mode; keep stores a
// successful answer. Done arms the next tick, so its Cmd must reach Bubble
// Tea even when the fetch failed.
func (m Model) onReply(tok async.Token, err error, keep func(Model) Model) (views.View, tea.Cmd) {
	if !m.poll.Accept(tok) {
		return m, nil
	}
	if fell, cmd, ok := m.fallBack(err); ok {
		return fell, cmd
	}
	var next tea.Cmd
	m.poll, next = m.poll.Done(tok, err)
	report := m.report(err)
	m.health = m.health.Track(err)
	m.loaded, m.asked = true, false
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	if err != nil {
		return m, tea.Batch(next, report)
	}
	m, gone := keep(m).withRows().landPending()
	return m, tea.Batch(next, gone)
}

// handleKey reports handled when the key matched a binding.
func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.toggleKey):
		next, cmd := m.toggle()
		return next, cmd, true
	case key.Matches(msg, m.sortKey):
		return m.withSort(otherSort(m.sort)).withRows(), nil, true
	case key.Matches(msg, m.watchKey):
		var cmd tea.Cmd
		m.poll, cmd = m.poll.WithEnabled(!m.poll.Enabled() && m.client != nil)
		return m, cmd, true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		var ok bool
		if m.poll, ok = m.poll.Begin(); !ok {
			return m, func() tea.Msg {
				return views.StatusMsg{Level: status.Info, Text: "refresh already running"}
			}, true
		}
		m.asked = true
		return m, m.fetch(), true
	}
	return m, nil, false
}

// toggle switches pods and nodes and fetches the new mode at once, under a
// fresh Token so the other mode's reply and tick are dropped.
func (m Model) toggle() (Model, tea.Cmd) {
	m = m.withMode(otherMode(m.mode))
	if m.client == nil {
		return m, nil
	}
	m.mint = m.mint.Next()
	m.poll = m.poll.Restart(m.mint)
	m.asked = true
	return m, m.fetch()
}

// withMode shows mode with an empty table until its first answer.
func (m Model) withMode(md mode) Model {
	m.mode = md
	m.table = newTable(md, m.namespace == "")
	m.pods, m.nodes, m.keys = nil, nil, nil
	m.loaded, m.health = false, m.health.Reset()
	m.pending = pendingRow{}
	return m
}

func otherMode(md mode) mode {
	if md == modeNodes {
		return modePods
	}
	return modeNodes
}

// Title implements views.View. Stable "top" so the navigation history
// can rebuild the view on Esc. The mode (pods vs nodes) is reflected in
// Command only.
func (m Model) Title() string { return "top" }

// Command implements views.View. The sort is always spelled out, so the
// command pasted from the footer orders its rows as the view does.
func (m Model) Command() kubectl.Cmd {
	sortBy := "--sort-by=" + string(m.sort)
	switch {
	case m.mode == modeNodes:
		return kubectl.New("top", "nodes", sortBy)
	case m.namespace != "":
		return kubectl.New("top", "pods", "-n", m.namespace, sortBy)
	}
	return kubectl.New("top", "pods", "-A", sortBy)
}

// Help implements views.View. The s and w hints show the current sort and
// the refresh cadence.
func (m Model) Help() []key.Binding {
	sortBy := m.sortKey
	sortBy.SetHelp("s", "sort "+string(m.sort))
	watch := m.watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{m.toggleKey, sortBy, watch, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
