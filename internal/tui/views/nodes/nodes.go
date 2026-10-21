// Package nodes is the nodes view: `kubectl get nodes -o wide` with NAME,
// STATUS (SchedulingDisabled on a cordoned node), ROLES (control-plane,
// etcd and master on k3s servers), AGE, VERSION, INTERNAL-IP, OS/ARCH and
// the taint count. Enter describes the node, o opens its YAML, / filters.
//
// The list refreshes through an async.Poller. Listing nodes takes a
// ClusterRole; a user without one is told which `kubectl auth can-i`
// checks it. A kubelet reports its own version and platform, so every
// cell is made inert before it is laid out.
package nodes

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
	// listTimeout is the poll deadline; a cluster has far fewer nodes than
	// pods, so one namespace's budget is enough.
	listTimeout = 5 * time.Second
)

var (
	describeKey = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "describe"))
	filterKey   = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter"))
	// Help labels w with the poller's cadence.
	watchKey   = key.NewBinding(key.WithKeys("w"))
	refreshKey = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh"))
)

// item is one listed node with its cells made inert, built once per list.
type item struct {
	name  string
	cells table.Row
}

// Model is the nodes view.
type Model struct {
	client *k8s.Client
	clock  async.Clock

	table table.Model
	items []item
	// shown are the names of the table's rows, in order.
	shown  []string
	health stale.Health
	loaded bool
	bodyH  int
	filter filter.Model

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

// New constructs a nodes view. With a client the first load is under
// way, and Init sends it.
func New(client *k8s.Client, opts ...Option) Model {
	m := Model{
		client: client,
		clock:  async.RealClock{},
		table:  newTable(),
		filter: filter.New(),
	}
	for _, opt := range opts {
		m = opt(m)
	}
	m.poll = async.NewPoller(async.New(), async.Config{
		Enabled: client != nil,
		Timeout: listTimeout,
		Clock:   m.clock,
	})
	if client != nil {
		m.poll, _ = m.poll.Begin()
	}
	return m
}

func newTable() table.Model {
	t := table.New(
		table.WithColumns(layout(0)),
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
func (m Model) Title() string { return "nodes" }

// Command implements views.View. The view shows OS/ARCH and the taint
// count where -o wide shows the OS image, kernel and runtime; the nodes
// are the same, so the command is exact.
func (m Model) Command() kubectl.Cmd { return kubectl.New("get", "nodes", "-o", "wide") }

// Help implements views.View. The w binding is labelled with the refresh
// cadence.
func (m Model) Help() []key.Binding {
	watch := watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{describeKey, filterKey, watch, refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. The view holds no stream or goroutine; a
// reply still in flight is dropped by its Token.
func (m Model) Close() error { return nil }
