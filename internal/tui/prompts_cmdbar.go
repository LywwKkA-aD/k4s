package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
)

// handleCmdBar dispatches based on the prompt mode. Container picker is a
// list and has its own keymap; help is a static popup that closes on any
// key; everything else (command, tail) shares the textinput-driven path.
func (m Model) handleCmdBar(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.cmdMode == cmdBarHelp {
		return m.closeCmdBar(), nil
	}
	if m.cmdMode == cmdBarContainerPrompt {
		return m.handleContainerPickerKey(msg)
	}
	if m.cmdMode == cmdBarRestorePrompt {
		return m.handleRestorePromptKey(msg)
	}
	switch {
	case keys.IsEsc(msg):
		return m.closeCmdBar(), nil
	case keys.IsEnter(msg):
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
				return updated.setError("port-forward: " + err.Error())
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

// cmdBarView renders the prompt bar. bubbles v2 pads a placeholder out to
// the width of a typed value plus the prompt; k4s has always fitted it,
// prompt included, into the input width, so an emptied prompt keeps the
// popup at the size it had.
func (m Model) cmdBarView() string {
	ti := m.cmdBar
	if ti.Value() == "" && ti.Placeholder != "" {
		ti.SetWidth(max(ti.Width()-lipgloss.Width(ti.Prompt)-1, 1))
	}
	return ti.View()
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

func (m Model) execCmd(input string) (Model, tea.Cmd) {
	name, ok := command.Resolve(input)
	if !ok {
		return m.setError(fmt.Sprintf("unknown command: %s", strings.TrimSpace(input)))
	}
	m.status = m.status.Clear()
	next := m.switchTo(name)
	return next, next.relayoutCmd()
}
