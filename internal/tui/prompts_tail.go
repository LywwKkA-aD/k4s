package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

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
