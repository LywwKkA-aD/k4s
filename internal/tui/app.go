// Package tui hosts the Bubble Tea application: the root model, view router,
// command bar and chrome (header + footer).
package tui

import (
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
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

// cmdBarMode tracks what the bottom input bar is currently being used for.
// "command" is the colon-driven view router; "tail" is the tail-lines prompt
// that opens before the logs view.
type cmdBarMode int

const (
	cmdBarOff cmdBarMode = iota
	cmdBarCommand
	cmdBarTailPrompt
	cmdBarContainerPrompt
	cmdBarForwardPrompt
	cmdBarRestorePrompt
	cmdBarHelp
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

	cmdBar  textinput.Model
	cmdMode cmdBarMode

	// status is the footer's status row; now is its clock and the grace
	// period's, time.Now unless a test sets WithClock.
	status status.Line
	now    func() time.Time

	// pendingTail* hold the parameters for a tail prompt currently in flight.
	// Cleared once the user submits or cancels.
	pendingTailNamespace string
	pendingTailPods      []string
	pendingTailContainer string

	// pendingContainer* hold the picker state. NextKind is "logs" or "exec";
	// the value chosen by the user is then translated into the appropriate
	// follow-up message. The picker is a list (not a textinput), so the
	// "value" is an index into pendingContainerContainers rather than the
	// cmdBar's typed string.
	pendingContainerNamespace  string
	pendingContainerPods       []string
	pendingContainerContainers []string
	pendingContainerNextKind   string
	pendingContainerIdx        int

	// pendingForward* hold the in-flight port-prompt context. Filled
	// when a list view emits ForwardRequestMsg and consumed when the
	// user confirms the local:remote pair.
	pendingForwardKind       string
	pendingForwardNamespace  string
	pendingForwardName       string
	pendingForwardRemotePort uint16

	// pendingRestoreCount is non-zero when the persisted state file
	// had entries at startup and we are still waiting for the user to
	// decide whether to revive them. Counted, not booled, so the popup
	// can render "Revive 3 forwards?" without re-walking the state.
	pendingRestoreCount int
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
func New(client *k8s.Client, loadErr error, opts ...Option) Model {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.SetWidth(40)
	ti.SetStyles(styles.TextInput())

	var mgr *forwards.Manager
	restore := 0
	if m, err := forwards.NewManager(client); err == nil {
		mgr = m
		restore = forwardsForContext(mgr.State().Forwards, currentContextName(client))
	}

	model := Model{
		client:              client,
		loadErr:             loadErr,
		forwardMgr:          mgr,
		keys:                keys.Default(),
		cmdBar:              ti,
		current:             dashboard.New(client, loadErr),
		pendingRestoreCount: restore,
		now:                 time.Now,
	}
	for _, opt := range opts {
		opt(&model)
	}
	// Open the restore popup automatically when the state file had
	// entries. The user can dismiss with n/Esc or revive everything
	// with y/Enter.
	if restore > 0 {
		model.cmdMode = cmdBarRestorePrompt
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
