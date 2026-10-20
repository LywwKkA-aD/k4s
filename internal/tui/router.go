package tui

import (
	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

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
		return m.onExecDone(msg)
	case cmdErrorClearMsg:
		if msg.value == m.cmdError {
			m.cmdError = ""
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m.routeToActiveUI(msg)
}

// onPaste delivers bracketed-paste text where typed text would go: the
// open text prompt, else the active view. Popups without a text field
// ignore it, except help, which closes on any input.
func (m Model) onPaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	switch m.cmdMode {
	case cmdBarOff:
		return m.forwardToView(msg)
	case cmdBarCommand, cmdBarTailPrompt, cmdBarForwardPrompt:
		var cmd tea.Cmd
		m.cmdBar, cmd = m.cmdBar.Update(msg)
		return m, cmd
	case cmdBarHelp:
		return m.closeCmdBar(), nil
	}
	return m, nil
}

// routeToActiveUI delivers a message that matched no root case. Pastes go
// where typed text would. Cursor blink ticks are not key messages, so while
// a text prompt is open they must reach the cmdBar explicitly — otherwise
// they fall through to the view and the cursor freezes after the first
// blink.
func (m Model) routeToActiveUI(msg tea.Msg) (tea.Model, tea.Cmd) {
	if paste, ok := msg.(tea.PasteMsg); ok {
		return m.onPaste(paste)
	}
	if m.cmdMode == cmdBarCommand || m.cmdMode == cmdBarTailPrompt || m.cmdMode == cmdBarForwardPrompt {
		if _, ok := msg.(cursor.BlinkMsg); ok {
			var cmd tea.Cmd
			m.cmdBar, cmd = m.cmdBar.Update(msg)
			return m, cmd
		}
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
		return m.setCmdError("context: " + err.Error())
	}
	if m.current != nil {
		_ = m.current.Close()
	}
	m.client = client
	m.loadErr = nil
	m.namespace = ""
	m.history = nil
	m.cmdError = ""
	// Tear down live forwards for the old cluster before swapping managers;
	// otherwise local ports stay bound and SPDY goroutines leak until exit.
	if m.forwardMgr != nil {
		_ = m.forwardMgr.StopAll()
	}
	var errCmd tea.Cmd
	if mgr, err := forwards.NewManager(client); err == nil {
		m.forwardMgr = mgr
	} else {
		m.forwardMgr = nil
		m, errCmd = m.setCmdError("forwards: " + err.Error())
	}
	m.current = dashboard.New(m.client, nil)
	return m, tea.Batch(m.relayoutCmd(), errCmd)
}

func (m Model) onNamespaceSelected(msg views.NamespaceSelectedMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m.namespace = msg.Namespace
	m = m.replaceView(viewPods)
	return m, m.relayoutCmd()
}

func (m Model) onDescribeRequest(msg views.DescribeRequestMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m = m.swap(describe.New(m.client, describe.Kind(msg.Kind), msg.Namespace, msg.Name))
	return m, m.relayoutCmd()
}

func (m Model) onLogsRequest(msg views.LogsRequestMsg) (tea.Model, tea.Cmd) {
	m = m.pushHistory()
	m = m.swap(logs.New(m.client, msg.Namespace, msg.Pods, msg.Tail, msg.Container))
	return m, m.relayoutCmd()
}

func (m Model) onKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if keys.IsForceQuit(msg) {
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
