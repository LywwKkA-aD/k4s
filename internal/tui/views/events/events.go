// Package events is the events view: `kubectl events -n <ns>` (or -A)
// with kubectl's columns LAST SEEN, TYPE, REASON, OBJECT and MESSAGE.
// kubectl prints the newest event last; the view puts it first, where
// the cursor starts. W keeps the warnings (--types=Warning), / filters,
// Enter describes the object an event is about and o opens the event's
// own YAML, where its whole message is.
//
// The list refreshes through an async.Poller and is bounded twice: the
// API is read in pages up to k8s.MaxEvents, and the table holds the
// newest maxRows of what W and the filter let through. Everything the
// cluster wrote is made inert before it is laid out: anyone who may
// create events chooses their reason, message and object.
//
// When -A is forbidden and the kubeconfig context names a namespace, the
// view lists that namespace instead and says so.
package events

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	minTableRows = 5

	// maxRows bounds the rows the table holds; the rest are counted in the
	// note above it.
	maxRows = 2000

	// modeWarnings is State.Mode while W keeps only the warnings.
	modeWarnings = "warnings"
	typeWarning  = "Warning"

	// Events across every namespace take longer to read, and may take
	// several pages; they get more time before the poller counts the API
	// as slow.
	namespaceTimeout     = 5 * time.Second
	allNamespacesTimeout = 15 * time.Second
)

var (
	describeKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe object"))
	warningsKey = key.NewBinding(key.WithKeys("W"))
	filterKey   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	// Help labels w with the poller's cadence and W with what it does next.
	watchKey   = key.NewBinding(key.WithKeys("w"))
	refreshKey = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
)

// item is one listed event with its cells made inert, built once per
// list so that filtering and drawing never touch the raw text.
type item struct {
	event k8s.Event
	cells table.Row
}

// Model is the events view of a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	// fellBack is set once -A was forbidden and the view moved to the
	// context's namespace.
	fellBack bool
	clock    async.Clock

	table table.Model
	width int
	items []item
	// shown are the events of the table's rows, in order; typed counts
	// the events W lets through and matched those the filter keeps too,
	// before maxRows cuts them.
	shown   []k8s.Event
	typed   int
	matched int
	// cut is set when the API held more than k8s.MaxEvents.
	cut      bool
	warnings bool

	health stale.Health
	loaded bool
	bodyH  int
	filter filter.Model

	// tok is the view's mint; the poller runs under it and moves to its
	// next Token when the namespace changes in place.
	tok  async.Token
	poll async.Poller
	// pendingKey is the row a restored State selected, until the first
	// list arrives to put the cursor on it; pendingCursor is where the
	// cursor goes if that row is gone.
	pendingKey    string
	pendingCursor int
}

// Option changes how New builds the view.
type Option func(Model) Model

// WithClock arms the refresh ticks on c instead of the real clock, so
// tests drive time by hand.
func WithClock(c async.Clock) Option {
	return func(m Model) Model {
		m.clock = c
		return m
	}
}

// New constructs an events view. With a client the first load is under
// way, and Init sends it.
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
	m.table = newTable(m.namespace == "")
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
		table.WithColumns(layout(0, showNamespace)),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)
	return t
}

// Init sends the first load; a view without a client has none.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	return m.list(k8s.FromCache())
}

// Title implements views.View. Stable routing name.
func (m Model) Title() string { return "events" }

// Command implements views.View: `kubectl events -n <ns>` or `-A`, with
// --types=Warning under W. It is approximate while the table holds fewer
// events than kubectl would print.
func (m Model) Command() kubectl.Cmd {
	args := []string{"events", "-A"}
	if m.namespace != "" {
		args = []string{"events", "-n", m.namespace}
	}
	if m.warnings {
		args = append(args, "--types="+typeWarning)
	}
	c := kubectl.New(args...)
	c.Approx = m.loaded && (m.cut || m.typed > maxRows)
	return c
}

// Help implements views.View. The w binding is labelled with the refresh
// cadence, W with what pressing it does.
func (m Model) Help() []key.Binding {
	watch := watchKey
	watch.SetHelp("w", m.poll.Hint())
	warn := warningsKey
	if m.warnings {
		warn.SetHelp("W", "all types")
	} else {
		warn.SetHelp("W", "warnings only")
	}
	return []key.Binding{describeKey, warn, filterKey, watch, refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. The view holds no stream or goroutine; a
// reply still in flight is dropped by its Token.
func (m Model) Close() error { return nil }
