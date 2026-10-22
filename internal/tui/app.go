// Package tui hosts the Bubble Tea application: the root model, view router,
// overlays (command bar, prompts, help) and chrome (header + footer).
package tui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/client-go/util/homedir"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

// View names used both for routing and for history entries.
const (
	viewDashboard   = "dashboard"
	viewPods        = "pods"
	viewNamespaces  = "namespaces"
	viewDeployments = "deployments"
	viewServices    = "services"
	viewEvents      = "events"
	viewNodes       = "nodes"
	viewContexts    = "contexts"
	viewTop         = "top"
	viewForwards    = "forwards"
	// The views of one object are history entries by these names and
	// their object; their titles name the object instead.
	viewDescribe = "describe"
	viewYAML     = "yaml"
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

	// startup is what startup found besides the client (see WithSession);
	// flags are the --context and --kubeconfig the user passed.
	startup startup.Session
	flags   sessionFlags
	// kubectlSession tells kubectl.Decorate how a pasted command reaches
	// this cluster; runner starts exec and describe with the client's
	// kubeconfig, and describer runs describe through it. All three follow
	// the client through :ctx. runnerConfig is the base every runner is
	// built from (see WithRunnerConfig).
	kubectlSession kubectl.Session
	runner         *kubectl.Runner
	runnerConfig   kubectl.RunnerConfig
	describer      describe.Kubectl

	namespace string // "" = all
	// usedNamespaces are the namespaces this session switched to, newest
	// first; with history they are what recentNamespaces offers.
	usedNamespaces []string
	current        views.View
	history        []historyEntry
	// build makes the view for a name; nil means newView. Tests use it to
	// put stub views in history.
	build func(m Model, name string) views.View

	// overlay is the open popup or prompt, nil when there is none. While
	// it is open it gets every key; what it asked for comes back as a
	// message once it closes.
	overlay overlay.Overlay

	// status is the footer's status row; now is its clock and the confirm
	// grace period's, time.Now unless a test sets WithClock.
	status status.Line
	now    func() time.Time

	// ident is what the header says about the cluster (header.go).
	ident identity
	// recap collects the commands main prints on exit (recap.go).
	recap recap
	// holdFrame keeps the first frame blank until the terminal says what
	// its background is (background.go).
	holdFrame bool
}

// Option changes how New builds the root model.
type Option func(*Model)

// WithClock makes the model read the time from now instead of time.Now, so
// tests can step through the status TTL and the confirm grace period.
func WithClock(now func() time.Time) Option {
	return func(m *Model) { m.now = now }
}

// WithRunnerConfig makes the session's kubectl runs (exec, describe) look
// kubectl up and build its environment through cfg; tests point it at a
// fake kubectl. The kubeconfig is always the client's: cfg.Kubeconfig is
// replaced.
func WithRunnerConfig(cfg kubectl.RunnerConfig) Option {
	return func(m *Model) { m.runnerConfig = cfg }
}

// New constructs the root model with the dashboard as the landing screen,
// in the client's starting namespace. loadErr is the kubeconfig load
// failure behind a nil client; the dashboard shows it instead of the
// cluster counters.
//
// The port-forward Manager is best-effort: if state loading fails (bad
// JSON, permission error) we keep the manager nil so the rest of k4s
// stays usable, and the forwards view will show an "unavailable" hint.
// When the state file holds forwards for this cluster, the model opens
// asking whether to revive them (restoreOverlay).
func New(client *k8s.Client, loadErr error, opts ...Option) Model {
	var mgr *forwards.Manager
	if m, err := forwards.NewManager(client); err == nil {
		mgr = m
	}

	model := Model{
		loadErr:    loadErr,
		forwardMgr: mgr,
		keys:       keys.Default(),
		current:    dashboard.New(client, loadErr),
		now:        time.Now,
	}
	for _, opt := range opts {
		opt(&model)
	}
	model = model.withClient(client)
	model.overlay = restoreOverlay(mgr, currentContextName(client), model.now)
	return model
}

// withClient makes c the active client: its starting namespace, the
// kubectl session footer commands are decorated with, and the runner exec
// and describe go through.
func (m Model) withClient(c *k8s.Client) Model {
	m.client = c
	m.ident = newIdentity(c)
	m.namespace = clientNamespace(c)
	m.kubectlSession = newKubectlSession(c, m.flags, os.Getenv, homedir.HomeDir())
	cfg := m.runnerConfig
	cfg.Kubeconfig = kubeconfigPaths(c)
	m.runner = kubectl.NewRunner(cfg)
	m.describer = newDescriber(c, m.runner, m.kubectlSession)
	return m
}

// newDescriber runs describe through r, decorated for session as the
// footer is. A client that names no kubeconfig files cannot point kubectl
// at its cluster (a bare kubectl could reach another one), so it gets the
// built-in describer alone.
func newDescriber(c *k8s.Client, r *kubectl.Runner, session kubectl.Session) describe.Kubectl {
	if len(kubeconfigPaths(c)) == 0 {
		return describe.Kubectl{}
	}
	return describe.NewKubectl(r, func(cmd kubectl.Cmd) kubectl.Cmd { return kubectl.Decorate(cmd, session) })
}

// Init starts the active view, asks the server for the header's version
// and the terminal for its background, and reports a failed preflight and
// a saved-forwards file that could not be read. A credential plugin the
// preflight did not log in with (--no-preflight, or a preflight that
// failed) gets the login probe :ctx runs (login.go).
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.current.Init(), m.fetchVersion(), m.askBackground()}
	if m.startup.ServerVersion == nil {
		cmds = append(cmds, loginProbe(m.client, false))
	}
	if err := m.startup.PreflightErr; err != nil {
		cmds = append(cmds, warnCmd(preflightText(err)))
	}
	if notice := forwardsNotice(m.forwardMgr); notice != "" {
		cmds = append(cmds, warnCmd(notice))
	}
	return tea.Batch(cmds...)
}

// warnCmd emits text as a warning for the status row.
func warnCmd(text string) tea.Cmd {
	msg := views.StatusMsg{Level: status.Warn, Text: text}
	return func() tea.Msg { return msg }
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
