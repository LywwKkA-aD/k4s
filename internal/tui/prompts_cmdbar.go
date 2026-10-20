package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
)

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
	if m.cmdMode == cmdBarRestorePrompt {
		return m.handleRestorePromptKey(msg)
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
				return updated.setCmdError("port-forward: " + err.Error())
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
		return m.setCmdError(fmt.Sprintf("unknown command: %s", strings.TrimSpace(input)))
	}
	m.cmdError = ""
	next := m.switchTo(name)
	return next, next.relayoutCmd()
}
