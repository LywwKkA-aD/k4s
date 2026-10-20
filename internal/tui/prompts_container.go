package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

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

// handleContainerPickerKey is the keymap for the container-picker popup —
// arrows / j-k navigate, Enter / 1..9 confirm, Esc cancels.
func (m Model) handleContainerPickerKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	n := len(m.pendingContainerContainers)
	if n == 0 {
		return m.closeCmdBar(), nil
	}

	switch {
	case keys.IsEsc(msg):
		return m.closeCmdBar(), nil
	case keys.IsEnter(msg):
		return m.commitContainerPick(m.pendingContainerIdx)
	case keys.IsArrow(msg, tea.KeyUp):
		m.pendingContainerIdx = (m.pendingContainerIdx - 1 + n) % n
		return m, nil
	case keys.IsArrow(msg, tea.KeyDown):
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
