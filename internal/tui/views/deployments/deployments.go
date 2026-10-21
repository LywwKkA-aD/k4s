// Package deployments is the deployments list view: rows show each deployment
// with kubectl-style READY / UP-TO-DATE / AVAILABLE counters. Enter opens
// describe; 'l' resolves the pods the deployment's ReplicaSets run and asks
// the root model to open the right prompt — single-container pod templates
// skip straight to the tail prompt, multi-container ones detour through the
// container picker first. Either way the logs view ends up streaming every
// running replica through the multi-pod machinery, with a coloured per-pod
// prefix.
//
// The list refreshes through an async.Poller, and Esc from describe or logs
// brings it back as it was left (views.Stateful and views.Restorer).
package deployments

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	minTableRows = 5
	// A list across every namespace may run longer than one namespace's
	// before the poller counts it as slow and backs off.
	namespaceTimeout     = 5 * time.Second
	allNamespacesTimeout = 15 * time.Second
)

// Option configures New.
type Option func(options) options

type options struct {
	clock async.Clock
}

// WithClock arms the view's timers on clock: the refresh tick and the
// "resolving logs" status. Tests pass an asynctest.Clock; nil keeps the
// real one.
func WithClock(c async.Clock) Option {
	return func(o options) options {
		if c != nil {
			o.clock = c
		}
		return o
	}
}

// Model is the deployments list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	clock     async.Clock

	table  table.Model
	filter filter.Model
	bodyH  int

	raw []k8s.Deployment
	// shown are the rows of the table, raw narrowed by the filter.
	shown []k8s.Deployment
	// health is the last list answer: an error over earlier rows is a stale
	// view, one before any rows is the body.
	health stale.Health
	loaded bool

	// tok is the mint row actions take their tokens from.
	tok   async.Token
	poll  async.Poller
	watch bool
	acts  async.InFlight
	// logsFor is the deployment the logs resolve in flight is for.
	logsFor views.ResourceRef
	// pending is a restored row waiting for the first list.
	pending pendingRow

	keys keyMap
}

type keyMap struct {
	describe key.Binding
	logs     key.Binding
	forward  key.Binding
	filter   key.Binding
	watch    key.Binding
	refresh  key.Binding
}

func newKeyMap() keyMap {
	return keyMap{
		describe: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe")),
		logs:     key.NewBinding(key.WithKeys("l"), key.WithHelp("l", "tail all replicas")),
		forward:  key.NewBinding(key.WithKeys("f"), key.WithHelp("f", "port-forward")),
		filter:   key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		watch:    key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watch")),
		refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}

// New constructs a deployments view for the given namespace ("" = all).
// With a client it starts the first list, which Init sends.
func New(client *k8s.Client, namespace string, opts ...Option) Model {
	o := options{clock: async.RealClock{}}
	for _, opt := range opts {
		o = opt(o)
	}
	t := table.New(
		table.WithColumns(tableColumns(namespace == "")),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)

	tok := async.New()
	poll := async.NewPoller(tok, async.Config{
		Enabled: client != nil,
		Timeout: listTimeout(namespace),
		Clock:   o.clock,
	})
	if client != nil {
		poll, _ = poll.Begin()
	}
	return Model{
		client:    client,
		namespace: namespace,
		clock:     o.clock,
		table:     t,
		filter:    filter.New(),
		tok:       tok,
		poll:      poll,
		watch:     true,
		keys:      newKeyMap(),
	}
}

func listTimeout(namespace string) time.Duration {
	if namespace == "" {
		return allNamespacesTimeout
	}
	return namespaceTimeout
}

// Init sends the first list New started; a restored view lists the same
// way, from the watch cache.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.list(k8s.FromCache())
}

// Update routes size, refresh, replies and keys, and forwards navigation
// to the table.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
		return m, nil
	case async.TickMsg:
		return m.onTick(msg)
	case async.Result[[]k8s.Deployment]:
		return m.onList(msg)
	case async.Result[k8s.LogTarget]:
		return m.onLogTarget(msg)
	case resolvingMsg:
		return m, m.onResolving(msg)
	case tea.PasteMsg:
		next, cmd, _ := m.feedFilter(msg)
		return next, cmd
	case tea.KeyPressMsg:
		if next, cmd, consumed := m.feedFilter(msg); consumed {
			return next, cmd
		}
		if next, cmd, handled := m.onKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading deployments…")
	}
	if m.health.Failed() {
		return styles.Warn.Render("deployments unavailable: " + m.health.Err().Error())
	}
	body := m.table.View()
	if len(m.shown) == 0 {
		if m.filter.Active() {
			body = styles.Hint.Render("no deployments match /" + m.filter.Query())
		} else {
			body = styles.Hint.Render("no deployments")
		}
	}
	above := make([]string, 0, 3)
	if m.filter.Active() {
		above = append(above, m.filter.View())
	}
	if banner := m.health.Banner(); banner != "" {
		above = append(above, banner)
	}
	if len(above) == 0 {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(above, body)...)
}

// Title implements views.View. Stable routing name.
func (m Model) Title() string { return "deployments" }

// Command implements views.View. k4s polls rather than holding a watch
// open, so the command has no --watch.
func (m Model) Command() kubectl.Cmd {
	if m.namespace == "" {
		return kubectl.New("get", "deployments", "-A")
	}
	return kubectl.New("get", "deployments", "-n", m.namespace)
}

// Help implements views.View. The w binding names the refresh cadence.
func (m Model) Help() []key.Binding {
	watch := m.keys.watch
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{m.keys.describe, m.keys.logs, m.keys.forward, m.keys.filter, watch, m.keys.refresh}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. No long-lived resources held: a reply or a
// tick still on its way is dropped by its token.
func (m Model) Close() error { return nil }
