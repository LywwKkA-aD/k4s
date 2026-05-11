// Package tui hosts the Bubble Tea application: the root model, view router,
// command bar and chrome (header + footer).
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
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
	cmdBarHelp
)

// relayoutMsg is an internal signal that says "re-send the current view a
// body-sized WindowSizeMsg" — used after view switches so freshly-mounted
// views are sized correctly without us re-entering tea.WindowSizeMsg, which
// would otherwise overwrite m.height with the smaller body height each time.
type relayoutMsg struct{}

// historyEntry captures the navigation snapshot we restore on Esc.
//
// Note: only "rebuildable" views (dashboard / pods / namespaces) end up here.
// Leaf views like describe push their parent on entry but never end up in
// history themselves, so popHistory always lands on something we can rebuild
// from {view, namespace}.
type historyEntry struct {
	view      string
	namespace string
}

// Model is the root Bubble Tea model.
type Model struct {
	client     *k8s.Client
	forwardMgr *forwards.Manager
	keys       keys.Map
	width      int
	height     int

	namespace string // "" = all
	current   views.View
	history   []historyEntry

	cmdBar   textinput.Model
	cmdMode  cmdBarMode
	cmdError string

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
}

// execDoneMsg is delivered by tea.ExecProcess after the kubectl-exec shell
// returns control to the TUI.
type execDoneMsg struct{ err error }

// New constructs the root model with the dashboard as the landing screen.
//
// The port-forward Manager is best-effort: if state loading fails (bad
// JSON, permission error) we keep the manager nil so the rest of k4s
// stays usable, and the forwards view will show an "unavailable" hint.
func New(client *k8s.Client) Model {
	ti := textinput.New()
	ti.CharLimit = 64
	ti.Width = 40

	var mgr *forwards.Manager
	if m, err := forwards.NewManager(client); err == nil {
		mgr = m
	}

	return Model{
		client:     client,
		forwardMgr: mgr,
		keys:       keys.Default(),
		cmdBar:     ti,
		current:    dashboard.New(client),
	}
}

// Init starts the active view.
func (m Model) Init() tea.Cmd {
	return m.current.Init()
}

// Update is the message router. Heavy lifting is split into per-message
// helpers so each one stays testable and gocyclo-friendly.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowResize(msg)
	case relayoutMsg:
		return m.onRelayout()
	case views.NamespaceSelectedMsg:
		return m.onNamespaceSelected(msg)
	case views.ContextSelectedMsg:
		return m.onContextSelected(msg)
	case views.DescribeRequestMsg:
		return m.onDescribeRequest(msg)
	case views.TailPromptRequestMsg:
		return m.onTailPromptRequest(msg)
	case views.ContainerPromptRequestMsg:
		return m.onContainerPromptRequest(msg)
	case views.LogsRequestMsg:
		return m.onLogsRequest(msg)
	case views.ExecRequestMsg:
		return m.onExecRequest(msg)
	case views.ForwardRequestMsg:
		return m.onForwardRequest(msg)
	case execDoneMsg:
		if msg.err != nil {
			m.cmdError = "exec: " + msg.err.Error()
		}
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m.forwardToView(msg)
}

func (m Model) onWindowResize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.height = msg.Height
	bodyMsg := tea.WindowSizeMsg{Width: msg.Width, Height: m.bodyHeight()}
	return m.forwardToView(bodyMsg)
}

func (m Model) onRelayout() (tea.Model, tea.Cmd) {
	if m.width == 0 || m.height == 0 {
		return m, nil
	}
	bodyMsg := tea.WindowSizeMsg{Width: m.width, Height: m.bodyHeight()}
	return m.forwardToView(bodyMsg)
}

// onContextSelected swaps the active k8s.Client to the chosen context. We
// drop the navigation history (its entries reference resources that may not
// exist in the new cluster) and the active namespace (likewise) and land
// the user back on the dashboard. Failures surface in the command-bar
// error line so the user gets feedback without losing their place — the
// previous client stays active.
func (m Model) onContextSelected(msg views.ContextSelectedMsg) (tea.Model, tea.Cmd) {
	client, err := k8s.LoadFromKubeconfigContext("", msg.Name)
	if err != nil {
		m.cmdError = "context: " + err.Error()
		return m, nil
	}
	if m.current != nil {
		_ = m.current.Close()
	}
	m.client = client
	m.namespace = ""
	m.history = nil
	m.cmdError = ""
	// Re-init the port-forward manager against the new cluster. Any
	// running forwards on the old client are abandoned — they were
	// pinned to a cluster the user no longer wants to look at.
	if mgr, err := forwards.NewManager(client); err == nil {
		m.forwardMgr = mgr
	}
	m.current = dashboard.New(m.client)
	return m, m.relayoutCmd()
}

func (m Model) onNamespaceSelected(msg views.NamespaceSelectedMsg) (tea.Model, tea.Cmd) {
	m.history = append(m.history, historyEntry{
		view:      m.current.Title(),
		namespace: m.namespace,
	})
	m.namespace = msg.Namespace
	m = m.replaceView(viewPods)
	return m, m.relayoutCmd()
}

func (m Model) onDescribeRequest(msg views.DescribeRequestMsg) (tea.Model, tea.Cmd) {
	m.history = append(m.history, historyEntry{
		view:      m.current.Title(),
		namespace: m.namespace,
	})
	m = m.swap(describe.New(m.client, describe.Kind(msg.Kind), msg.Namespace, msg.Name))
	return m, m.relayoutCmd()
}

// onTailPromptRequest opens the tail-lines prompt prefilled with 100. When
// the user submits, handleCmdBar dispatches a LogsRequestMsg with the parsed
// number and the resolved container; the actual logs view is created in
// onLogsRequest.
func (m Model) onTailPromptRequest(msg views.TailPromptRequestMsg) (tea.Model, tea.Cmd) {
	m.cmdMode = cmdBarTailPrompt
	m.pendingTailNamespace = msg.Namespace
	m.pendingTailPods = msg.Pods
	m.pendingTailContainer = msg.Container
	m.cmdBar.Prompt = "tail lines: "
	m.cmdBar.Placeholder = "100"
	m.cmdBar.SetValue("100")
	m.cmdBar.CursorEnd()
	m.cmdBar.Focus()
	return m, textinput.Blink
}

// onContainerPromptRequest opens the container picker as a list popup.
// Unlike the tail prompt this is not a textinput — the user navigates with
// arrows / j-k and confirms with Enter (or jumps to a slot with 1..9).
func (m Model) onContainerPromptRequest(msg views.ContainerPromptRequestMsg) (tea.Model, tea.Cmd) {
	if len(msg.Containers) == 0 {
		return m, nil
	}
	m.cmdMode = cmdBarContainerPrompt
	m.pendingContainerNamespace = msg.Namespace
	m.pendingContainerPods = msg.Pods
	m.pendingContainerContainers = msg.Containers
	m.pendingContainerNextKind = msg.NextKind
	m.pendingContainerIdx = 0
	return m, nil
}

func (m Model) onLogsRequest(msg views.LogsRequestMsg) (tea.Model, tea.Cmd) {
	m.history = append(m.history, historyEntry{
		view:      m.current.Title(),
		namespace: m.namespace,
	})
	m = m.swap(logs.New(m.client, msg.Namespace, msg.Pods, msg.Tail, msg.Container))
	return m, m.relayoutCmd()
}

// onForwardRequest opens the port prompt prefilled with "remote:remote"
// when we know the remote port, or "8080:" when we don't — so the user
// usually only has to press Enter. The actual port-forward fires from
// the submission path via dispatchForwardCmd.
func (m Model) onForwardRequest(msg views.ForwardRequestMsg) (tea.Model, tea.Cmd) {
	if m.forwardMgr == nil {
		m.cmdError = "port-forward: state subsystem unavailable"
		return m, nil
	}
	m.cmdMode = cmdBarForwardPrompt
	m.pendingForwardKind = msg.Kind
	m.pendingForwardNamespace = msg.Namespace
	m.pendingForwardName = msg.Name
	m.pendingForwardRemotePort = msg.RemotePort
	m.cmdBar.Prompt = fmt.Sprintf("port-forward %s/%s — local:remote: ", msg.Kind, msg.Name)
	placeholder := "8080:80"
	prefill := ""
	if msg.RemotePort > 0 {
		placeholder = fmt.Sprintf("%d:%d", msg.RemotePort, msg.RemotePort)
		prefill = placeholder
	}
	m.cmdBar.Placeholder = placeholder
	m.cmdBar.SetValue(prefill)
	m.cmdBar.CursorEnd()
	m.cmdBar.Focus()
	return m, textinput.Blink
}

// onExecRequest runs `kubectl exec -it` against the named pod / container
// via tea.ExecProcess so the shell takes over the terminal cleanly. When
// the shell exits we get an execDoneMsg back at the root.
func (m Model) onExecRequest(msg views.ExecRequestMsg) (tea.Model, tea.Cmd) {
	cmd := buildExecCommand(msg.Namespace, msg.Pod, msg.Container)
	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

// buildExecCommand returns the kubectl exec command. Extracted so the args
// are unit-testable without spawning a subprocess.
//
// gosec G204: exec.Command with variable args is normally a red flag, but
// here every variable comes from a Kubernetes API object (namespace, pod,
// container names that the user already had RBAC to read). None of it
// originates from a free-form text field — even the container name is
// matched back against the API-supplied list before this is called.
func buildExecCommand(namespace, pod, container string) *exec.Cmd {
	args := []string{"exec", "-it", "-n", namespace}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, pod, "--", "sh")
	return exec.Command("kubectl", args...) //nolint:gosec
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if m.cmdMode != cmdBarOff {
		return m.handleCmdBar(msg)
	}
	// Views that own the keyboard (focused textinput, list-popup, etc.) get
	// every keystroke first. Without this, 'q' inside a filter prompt would
	// be intercepted as "go home" before it ever reached the view.
	if m.current != nil && m.current.CapturesKeys() {
		return m.forwardToView(msg)
	}
	switch {
	case key.Matches(msg, m.keys.Quit):
		if m.current.Title() == viewDashboard {
			return m, tea.Quit
		}
		next := m.goHome()
		return next, next.relayoutCmd()
	case key.Matches(msg, m.keys.Command):
		m.cmdMode = cmdBarCommand
		m.cmdError = ""
		m.cmdBar.Prompt = ": "
		m.cmdBar.Placeholder = "pods, ns, dashboard"
		m.cmdBar.SetValue("")
		m.cmdBar.Focus()
		return m, textinput.Blink
	case key.Matches(msg, m.keys.Help):
		m.cmdMode = cmdBarHelp
		return m, nil
	case key.Matches(msg, m.keys.Back):
		if next, ok := m.popHistory(); ok {
			return next, next.relayoutCmd()
		}
		return m, nil
	}
	return m.forwardToView(msg)
}

func (m Model) forwardToView(msg tea.Msg) (tea.Model, tea.Cmd) {
	upd, cmd := m.current.Update(msg)
	if v, ok := upd.(views.View); ok {
		m.current = v
	}
	return m, cmd
}

// relayoutCmd batches the new view's Init() with a relayoutMsg so the
// freshly-mounted view is sized correctly before its first paint —
// otherwise it briefly renders at zero width/height after a switch.
//
// We use the internal relayoutMsg type rather than tea.WindowSizeMsg so the
// app does not mistake our synthetic resize for a real terminal resize and
// shrink m.height by the chrome height on every view switch.
func (m Model) relayoutCmd() tea.Cmd {
	initCmd := m.current.Init()
	if m.width == 0 && m.height == 0 {
		return initCmd
	}
	resize := func() tea.Msg { return relayoutMsg{} }
	if initCmd == nil {
		return resize
	}
	return tea.Batch(initCmd, resize)
}

// bodyHeight returns the number of lines available between header and footer,
// computed from the *current* chrome so we match whatever lipgloss actually
// rendered.
func (m Model) bodyHeight() int {
	hh := lipgloss.Height(m.renderHeader())
	fh := lipgloss.Height(m.renderFooter())
	h := m.height - hh - fh
	if h < 1 {
		return 1
	}
	return h
}

// handleCmdBar dispatches based on the prompt mode. Container picker is a
// list and has its own keymap; help is a static popup that closes on any
// key; everything else (command, tail) shares the textinput-driven path.
func (m Model) handleCmdBar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.cmdMode == cmdBarHelp {
		return m.closeCmdBar(), nil
	}
	if m.cmdMode == cmdBarContainerPrompt {
		return m.handleContainerPickerKey(msg)
	}
	switch msg.Type {
	case tea.KeyEsc:
		return m.closeCmdBar(), nil
	case tea.KeyEnter:
		value := m.cmdBar.Value()
		mode := m.cmdMode
		// Snapshot the pending-* context BEFORE closeCmdBar wipes it.
		tailNS, tailPods, tailContainer := m.pendingTailNamespace, m.pendingTailPods, m.pendingTailContainer
		fwdKind, fwdNS, fwdName, fwdRemote := m.pendingForwardKind, m.pendingForwardNamespace, m.pendingForwardName, m.pendingForwardRemotePort
		mgr := m.forwardMgr
		next := m.closeCmdBar()
		switch mode {
		case cmdBarCommand:
			return next.execCmd(value)
		case cmdBarTailPrompt:
			return next, dispatchTailCmd(tailNS, tailPods, tailContainer, value)
		case cmdBarForwardPrompt:
			updated, err := dispatchForward(next, mgr, fwdKind, fwdNS, fwdName, fwdRemote, value)
			if err != nil {
				updated.cmdError = "port-forward: " + err.Error()
				return updated, nil
			}
			// Hop to the forwards view so the user sees the entry land.
			next2 := updated.switchTo(viewForwards)
			return next2, next2.relayoutCmd()
		}
		return next, nil
	}
	var cmd tea.Cmd
	m.cmdBar, cmd = m.cmdBar.Update(msg)
	return m, cmd
}

// handleContainerPickerKey is the keymap for the container-picker popup —
// arrows / j-k navigate, Enter / 1..9 confirm, Esc cancels.
func (m Model) handleContainerPickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(m.pendingContainerContainers)
	if n == 0 {
		return m.closeCmdBar(), nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		return m.closeCmdBar(), nil
	case tea.KeyEnter:
		return m.commitContainerPick(m.pendingContainerIdx)
	case tea.KeyUp:
		m.pendingContainerIdx = (m.pendingContainerIdx - 1 + n) % n
		return m, nil
	case tea.KeyDown:
		m.pendingContainerIdx = (m.pendingContainerIdx + 1) % n
		return m, nil
	}

	switch msg.String() {
	case "k":
		m.pendingContainerIdx = (m.pendingContainerIdx - 1 + n) % n
		return m, nil
	case "j":
		m.pendingContainerIdx = (m.pendingContainerIdx + 1) % n
		return m, nil
	}

	// Quick-pick: digit 1..9 confirms the slot at that index, ignored if
	// out of range. Only single-digit keys are considered.
	if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0] - '1')
		if idx < n {
			return m.commitContainerPick(idx)
		}
	}
	return m, nil
}

// commitContainerPick fires the dispatching cmd for the chosen container
// and tears down the picker state.
func (m Model) commitContainerPick(idx int) (tea.Model, tea.Cmd) {
	chosen := m.pendingContainerContainers[idx]
	ns := m.pendingContainerNamespace
	pods := m.pendingContainerPods
	nextKind := m.pendingContainerNextKind
	next := m.closeCmdBar()
	return next, dispatchContainerCmd(ns, pods, chosen, nextKind)
}

// closeCmdBar resets the prompt bar and clears any prompt-specific state.
// Always go through this on Esc / Enter so pending* fields don't linger.
func (m Model) closeCmdBar() Model {
	m.cmdMode = cmdBarOff
	m.cmdBar.Reset()
	m.cmdBar.Blur()
	m.cmdBar.Prompt = ": "
	m.cmdBar.Placeholder = ""
	m.pendingTailNamespace = ""
	m.pendingTailPods = nil
	m.pendingTailContainer = ""
	m.pendingContainerNamespace = ""
	m.pendingContainerPods = nil
	m.pendingContainerContainers = nil
	m.pendingContainerNextKind = ""
	m.pendingContainerIdx = 0
	m.pendingForwardKind = ""
	m.pendingForwardNamespace = ""
	m.pendingForwardName = ""
	m.pendingForwardRemotePort = 0
	return m
}

// dispatchForward parses "local:remote" (or "local" with a known remote)
// and registers the forward with the Manager, then kicks off Start in a
// background goroutine — the Manager updates the UI via its change
// channel. We do **not** make this a tea.Cmd because Start may block
// briefly while resolving the target pod, which is fine off the loop.
func dispatchForward(m Model, mgr *forwards.Manager, kind, ns, name string, remoteHint uint16, raw string) (Model, error) {
	if mgr == nil {
		return m, fmt.Errorf("subsystem unavailable")
	}
	local, remote, err := parsePortPair(strings.TrimSpace(raw), remoteHint)
	if err != nil {
		return m, err
	}
	fwd := forwards.Forward{
		ID:         forwards.NewID(),
		Context:    currentContextName(m.client),
		Namespace:  ns,
		Kind:       kind,
		Name:       name,
		LocalPort:  local,
		RemotePort: remote,
	}
	if err := mgr.Register(fwd); err != nil {
		return m, err
	}
	go func(id string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mgr.Start(ctx, id)
	}(fwd.ID)
	return m, nil
}

// parsePortPair accepts "local:remote", "local" (uses remoteHint), or
// ":remote" (local := remote). Returns a clear error for anything else
// so the user sees a useful message instead of a port-forward that
// silently fails.
func parsePortPair(raw string, remoteHint uint16) (local, remote uint16, err error) {
	if raw == "" {
		if remoteHint == 0 {
			return 0, 0, fmt.Errorf("port required (try 8080:80)")
		}
		return remoteHint, remoteHint, nil
	}
	parts := strings.SplitN(raw, ":", 2)
	switch len(parts) {
	case 1:
		l, perr := parsePort(parts[0])
		if perr != nil {
			return 0, 0, perr
		}
		if remoteHint == 0 {
			return l, l, nil
		}
		return l, remoteHint, nil
	case 2:
		var l, r uint16
		var perr error
		if parts[0] != "" {
			l, perr = parsePort(parts[0])
			if perr != nil {
				return 0, 0, fmt.Errorf("local port: %w", perr)
			}
		}
		if parts[1] != "" {
			r, perr = parsePort(parts[1])
			if perr != nil {
				return 0, 0, fmt.Errorf("remote port: %w", perr)
			}
		}
		switch {
		case l == 0 && r == 0:
			return 0, 0, fmt.Errorf("both ports empty")
		case l == 0:
			return r, r, nil
		case r == 0:
			return l, l, nil
		}
		return l, r, nil
	}
	return 0, 0, fmt.Errorf("invalid format: %q", raw)
}

func parsePort(s string) (uint16, error) {
	n, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("%q is not a port number", s)
	}
	if n == 0 {
		return 0, fmt.Errorf("port must be > 0")
	}
	return uint16(n), nil
}

// dispatchTailCmd parses the tail-prompt value and returns a Cmd that emits
// LogsRequestMsg with the chosen tail. Invalid / non-positive input falls
// back to 100 — a casual prompt should never surface a hard error.
func dispatchTailCmd(ns string, pods []string, container, value string) tea.Cmd {
	n, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || n <= 0 {
		n = 100
	}
	return func() tea.Msg {
		return views.LogsRequestMsg{Namespace: ns, Pods: pods, Tail: n, Container: container}
	}
}

// dispatchContainerCmd routes the user's container choice to the right
// follow-up message based on what the picker was opened for.
func dispatchContainerCmd(ns string, pods []string, container, nextKind string) tea.Cmd {
	switch nextKind {
	case "logs":
		return func() tea.Msg {
			return views.TailPromptRequestMsg{Namespace: ns, Pods: pods, Container: container}
		}
	case "exec":
		if len(pods) == 0 {
			return nil
		}
		return func() tea.Msg {
			return views.ExecRequestMsg{Namespace: ns, Pod: pods[0], Container: container}
		}
	}
	return nil
}

func (m Model) execCmd(input string) (Model, tea.Cmd) {
	name, ok := command.Resolve(input)
	if !ok {
		m.cmdError = fmt.Sprintf("unknown command: %s", strings.TrimSpace(input))
		return m, nil
	}
	m.cmdError = ""
	next := m.switchTo(name)
	return next, next.relayoutCmd()
}

// switchTo records the current view in history then constructs the named view.
func (m Model) switchTo(name string) Model {
	if m.current != nil && m.current.Title() != name {
		m.history = append(m.history, historyEntry{
			view:      m.current.Title(),
			namespace: m.namespace,
		})
	}
	return m.replaceView(name)
}

// replaceView builds the named view without touching history.
func (m Model) replaceView(name string) Model {
	switch name {
	case viewPods:
		return m.swap(pods.New(m.client, m.namespace))
	case viewNamespaces:
		return m.swap(namespaces.New(m.client))
	case viewDeployments:
		return m.swap(deployments.New(m.client, m.namespace))
	case viewServices:
		return m.swap(services.New(m.client, m.namespace))
	case viewDashboard:
		return m.swap(dashboard.New(m.client))
	case viewContexts:
		return m.swap(contexts.New(currentContextName(m.client)))
	case viewTop:
		return m.swap(top.New(m.client, m.namespace))
	case viewForwards:
		return m.swap(forwardsview.New(m.forwardMgr))
	}
	return m
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

// swap closes the outgoing view (so its goroutines / streams are stopped)
// and installs the incoming one.
func (m Model) swap(next views.View) Model {
	if m.current != nil {
		_ = m.current.Close()
	}
	m.current = next
	return m
}

// goHome resets history and returns to the dashboard.
func (m Model) goHome() Model {
	m.history = nil
	return m.replaceView(viewDashboard)
}

// popHistory restores the previous view + namespace; returns false if empty.
func (m Model) popHistory() (Model, bool) {
	if len(m.history) == 0 {
		return m, false
	}
	last := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.namespace = last.namespace
	return m.replaceView(last.view), true
}

// View composes header / body / footer. The body normally renders the
// active view, but multi-field prompts (tail lines, container picker) are
// drawn as centered popups that temporarily cover the view — the single-
// line ":" command bar stays in the footer because it is one line by
// design and a popup would feel heavyweight.
func (m Model) View() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	bodyHeight := m.bodyHeight()
	bodyWidth := max(m.width, 0)

	content := m.current.View()
	if m.cmdMode == cmdBarTailPrompt || m.cmdMode == cmdBarContainerPrompt || m.cmdMode == cmdBarHelp {
		content = m.renderPromptPopup(bodyWidth, bodyHeight)
	}

	body := lipgloss.NewStyle().
		Width(bodyWidth).
		Height(bodyHeight).
		MaxHeight(bodyHeight).
		Render(content)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// renderPromptPopup renders the active prompt as a centred bordered card.
// Tail prompt is a textinput; container prompt is a navigable list.
func (m Model) renderPromptPopup(width, height int) string {
	var inner string
	switch m.cmdMode {
	case cmdBarTailPrompt:
		inner = lipgloss.JoinVertical(
			lipgloss.Left,
			styles.PopupTitle.Render("tail lines"),
			"",
			m.cmdBar.View(),
			"",
			styles.Hint.Render("Enter run · Esc cancel · default 100"),
		)
	case cmdBarContainerPrompt:
		inner = m.renderContainerPickerInner()
	case cmdBarHelp:
		inner = m.renderHelpPopupInner()
	default:
		return ""
	}
	box := styles.PopupBox.Render(inner)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// renderContainerPickerInner builds the body of the container-picker popup:
// a numbered list with the current selection highlighted in the accent
// colour, plus a hint line.
func (m Model) renderContainerPickerInner() string {
	containers := m.pendingContainerContainers
	rows := make([]string, 0, len(containers)+4)
	rows = append(rows, styles.PopupTitle.Render("container"), "")
	for i, name := range containers {
		marker := "  "
		label := fmt.Sprintf("%d. %s", i+1, name)
		if i == m.pendingContainerIdx {
			marker = styles.OK.Render("▶ ")
			label = styles.OK.Render(label)
		}
		rows = append(rows, marker+label)
	}
	rows = append(rows, "", styles.Hint.Render("↑↓/jk navigate · 1-9 quick · Enter pick · Esc cancel"))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func (m Model) renderHeader() string {
	parts := []string{styles.Title.Render("k4s")}
	if m.client != nil {
		parts = append(parts, styles.Hint.Render("ctx: "+m.client.Context))
	}
	ns := m.namespace
	if ns == "" {
		ns = "ALL"
	}
	parts = append(parts,
		styles.Hint.Render("ns: "+ns),
		styles.Hint.Render("view: "+m.current.Title()),
	)

	w := max(m.width-2, 0)
	return styles.Header.Width(w).Render(strings.Join(parts, "  ·  "))
}

func (m Model) renderFooter() string {
	w := max(m.width-2, 0)

	// Only the single-line ":" command renders inline in the footer.
	// Multi-field prompts (tail / container) live in a centred popup
	// drawn in the body, so the footer stays informative.
	if m.cmdMode == cmdBarCommand {
		return styles.Footer.Width(w).Render(m.cmdBar.View())
	}

	var top string
	if m.cmdError != "" {
		top = styles.Warn.Render(m.cmdError)
	} else {
		top = styles.Hint.Render(strings.Join(m.footerBindings(), "  ·  "))
	}

	bottom := ""
	if eq := m.current.KubectlEquivalent(); eq != "" {
		bottom = styles.KubectlHint.Render("≈ " + eq)
	}

	if bottom == "" {
		return styles.Footer.Width(w).Render(top)
	}
	return styles.Footer.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) footerBindings() []string {
	quitLabel := "q quit"
	if m.current.Title() != viewDashboard {
		quitLabel = "q home"
	}
	bindings := []string{quitLabel, "^c quit", "esc back", ": command", "? help"}
	for _, b := range m.current.Help() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			bindings = append(bindings, h.Key+" "+h.Desc)
		}
	}
	return bindings
}
