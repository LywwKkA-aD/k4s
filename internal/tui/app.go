// Package tui hosts the Bubble Tea application: the root model, view router,
// overlays (command bar, prompts, help) and chrome (header + footer).
package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// View names used both for routing and for history entries.
const (
	viewDashboard   = "dashboard"
	viewPods        = "pods"
	viewNamespaces  = "namespaces"
	viewDeployments = "deployments"
	viewServices    = "services"
	viewContexts    = "contexts"
	viewTop         = "top"
	viewForwards    = "forwards"
)

// relayoutMsg is an internal signal that says "re-send the current view a
// body-sized WindowSizeMsg" — used after view switches so freshly-mounted
// views are sized correctly without us re-entering tea.WindowSizeMsg, which
// would otherwise overwrite m.height with the smaller body height each time.
type relayoutMsg struct{}

// Model is the root Bubble Tea model.
type Model struct {
	client *k8s.Client
	// loadErr is why client is nil at startup; cleared once a context
	// switch produces a working client.
	loadErr    error
	forwardMgr *forwards.Manager
	keys       keys.Map
	width      int
	height     int

	namespace string // "" = all
	current   views.View
	history   []historyEntry

	// overlay is the open popup or prompt, nil when there is none. While
	// it is open it gets every key; what it asked for comes back as a
	// message once it closes.
	overlay overlay.Overlay

	// status is the footer's status row; now is its clock and the confirm
	// grace period's, time.Now unless a test sets WithClock.
	status status.Line
	now    func() time.Time
}

// Option changes how New builds the root model.
type Option func(*Model)

// WithClock makes the model read the time from now instead of time.Now, so
// tests can step through the status TTL and the confirm grace period.
func WithClock(now func() time.Time) Option {
	return func(m *Model) { m.now = now }
}

// New constructs the root model with the dashboard as the landing screen.
// loadErr is the kubeconfig load failure behind a nil client; the dashboard
// shows it instead of the cluster counters.
//
// The port-forward Manager is best-effort: if state loading fails (bad
// JSON, permission error) we keep the manager nil so the rest of k4s
// stays usable, and the forwards view will show an "unavailable" hint.
// When the state file holds forwards for the current context, the model
// opens asking whether to revive them.
func New(client *k8s.Client, loadErr error, opts ...Option) Model {
	var mgr *forwards.Manager
	restore := 0
	if m, err := forwards.NewManager(client); err == nil {
		mgr = m
		restore = forwardsForContext(mgr.State().Forwards, currentContextName(client))
	}

	model := Model{
		client:     client,
		loadErr:    loadErr,
		forwardMgr: mgr,
		keys:       keys.Default(),
		current:    dashboard.New(client, loadErr),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(&model)
	}
	if restore > 0 {
		model.overlay = overlay.NewRestore(restore, model.now)
	}
	return model
}

// Init starts the active view.
func (m Model) Init() tea.Cmd {
	return m.current.Init()
}

// currentContextName is a small helper so the contexts view can render the
// "★ current" marker without us reaching into k8s.Client at the call site
// (and gracefully handling the nil-client case before kubeconfig is loaded).
func currentContextName(c *k8s.Client) string {
	if c == nil {
		return ""
	}
	return c.Context
}
