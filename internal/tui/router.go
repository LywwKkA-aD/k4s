package tui

import (
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
	yamlview "github.com/LywwKkA-aD/k4s/internal/tui/views/yaml"
)

// Update is the message router. Heavy lifting is split into per-message
// helpers so each one stays testable and gocyclo-friendly.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msg = keys.Normalize(msg)
	m = m.noteRecap(msg)
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowResize(msg)
	case relayoutMsg:
		return m.onRelayout()
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.PasteMsg, cursor.BlinkMsg:
		return m.onInput(msg)
	case selfApplyingMsg:
		return m.onSelfApplying(msg)
	case views.StatusMsg:
		return m.setStatus(msg.Level, msg.Text)
	case status.ExpireMsg:
		m.status = m.status.Expire(msg, m.now())
		return m, nil
	}
	if next, ok := m.onChromeMsg(msg); ok {
		return next, nil
	}
	if next, cmd, ok := m.onRequest(msg); ok {
		return next, cmd
	}
	if next, cmd, ok := m.onOverlayResult(msg); ok {
		return next, cmd
	}
	return m.forwardToView(msg)
}

// selfApplyingMsg is the reply to work the root started itself, which
// knows how to apply itself to the model: the steps of the flows it runs
// around the terminal (the pod lookup before `kubectl exec` and the shell
// that came back, exec.go; the login flow after :ctx, login.go) and the
// header's /version request with its retries (header_version.go). Each
// carries what it needs to tell a reply for another client or context and
// drop it. The router keeps the Cmd a self-applying reply returns, so one
// can arm a retry.
type selfApplyingMsg interface {
	apply(m Model) (tea.Model, tea.Cmd)
}

// onSelfApplying applies a reply to work the root started.
func (m Model) onSelfApplying(msg selfApplyingMsg) (tea.Model, tea.Cmd) {
	return msg.apply(m)
}

// onRequest handles what a view asks the root to do; ok is false for any
// other message.
func (m Model) onRequest(msg tea.Msg) (next tea.Model, cmd tea.Cmd, ok bool) {
	switch msg := msg.(type) {
	case views.NamespaceSelectedMsg:
		next, cmd = m.onNamespaceSelected(msg)
	case views.NamespaceFallbackMsg:
		next = m.onNamespaceFallback(msg)
	case views.ContextSelectedMsg:
		next, cmd = m.onContextSelected(msg)
	case views.DescribeRequestMsg:
		next, cmd = m.onDescribeRequest(msg)
	case views.YAMLRequestMsg:
		next, cmd = m.onYAMLRequest(msg)
	case views.TailPromptRequestMsg:
		next = m.onTailPromptRequest(msg)
	case views.ContainerPromptRequestMsg:
		next = m.onContainerPromptRequest(msg)
	case views.LogsRequestMsg:
		next, cmd = m.onLogsRequest(msg)
	case views.ExecRequestMsg:
		next, cmd = m.onExecRequest(msg)
	case views.ForwardRequestMsg:
		next, cmd = m.onForwardRequest(msg)
	default:
		return m, nil, false
	}
	return next, cmd, true
}

// onInput delivers a paste or a cursor blink where typed text goes: the open
// overlay, else the active view. Blinks are not key messages, so without
// this an overlay's cursor would freeze after its first blink.
func (m Model) onInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.overlay != nil {
		return m.onOverlayInput(msg)
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

func (m Model) onNamespaceSelected(msg views.NamespaceSelectedMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m = m.enterNamespace(msg.Namespace)
	m = m.replaceView(viewPods)
	return m, m.relayoutCmd()
}

// onNamespaceFallback follows a list that fell back from every namespace
// to msg.Namespace: the header and every history entry pushed from now on
// name it, and the view, which already lists it, stays. A fallback that
// arrives once the root is in a namespace, or the list is gone, is stale.
func (m Model) onNamespaceFallback(msg views.NamespaceFallbackMsg) Model {
	if m.namespace != "" || !isNamespacedList(m.current.Title()) {
		return m
	}
	return m.enterNamespace(msg.Namespace)
}

func (m Model) onDescribeRequest(msg views.DescribeRequestMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m = m.swap(m.describeView(views.ResourceRef(msg)))
	return m, m.relayoutCmd()
}

// openYAML is o: the YAML of the object the current view shows or has
// under its cursor, or a status that says why there is none.
func (m Model) openYAML() (tea.Model, tea.Cmd) {
	sel, ok := m.current.(views.Selection)
	if !ok {
		return m.setStatus(status.Info, "yaml: nothing to show here")
	}
	ref, ok := sel.Selected()
	switch {
	case ok:
		return m.onYAMLRequest(views.YAMLRequestMsg{Ref: ref})
	case m.client == nil:
		return m.setStatus(status.Warn, "yaml: no kubeconfig")
	}
	return m.setStatus(status.Info, "yaml: no row selected")
}

// onYAMLRequest opens the YAML view of msg.Ref. Asked again for the YAML
// already shown, it fetches it again in place rather than stacking a copy
// under Esc.
func (m Model) onYAMLRequest(msg views.YAMLRequestMsg) (tea.Model, tea.Cmd) {
	if e, ok := m.entryOf(m.current); !ok || e.view != viewYAML || e.ref != msg.Ref {
		m = m.pushHistory()
	}
	m = m.swap(yamlview.New(m.client, msg.Ref))
	return m, m.relayoutCmd()
}

func (m Model) onLogsRequest(msg views.LogsRequestMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m = m.swap(logs.New(m.client, msg.Namespace, msg.Pods, msg.Tail, msg.Container, logs.WithTarget(msg.Logs)))
	return m, m.relayoutCmd()
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if keys.IsForceQuit(msg) {
		return m, tea.Quit
	}
	m.status = m.status.KeyPressed()
	if m.overlay != nil {
		// y copies over any overlay without a text input; the overlay stays.
		if key.Matches(msg, m.keys.Copy) && !overlay.TakesText(m.overlay) {
			return m.copyCommand()
		}
		return m.onOverlayInput(msg)
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
		return m.openCommandBar(), nil
	case key.Matches(msg, m.keys.Help):
		return m.openHelp(), nil
	case key.Matches(msg, m.keys.Back):
		return m.back()
	case key.Matches(msg, m.keys.Copy):
		return m.copyCommand()
	case key.Matches(msg, m.keys.YAML):
		return m.openYAML()
	}
	return m.forwardToView(msg)
}

func (m Model) forwardToView(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.current, cmd = m.current.Update(msg)
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
