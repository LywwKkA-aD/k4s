package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onTailPromptRequest opens the tail-lines prompt, which names the request's
// target. Its answer becomes a LogsRequestMsg; the logs view itself is
// created in onLogsRequest.
func (m Model) onTailPromptRequest(msg views.TailPromptRequestMsg) Model {
	m.overlay = overlay.NewTail(msg)
	return m
}

// logsRequestCmd asks for the logs the tail prompt was answered for.
func logsRequestCmd(msg overlay.TailChosenMsg) tea.Cmd {
	req := msg.Request
	return func() tea.Msg {
		return views.LogsRequestMsg{Namespace: req.Namespace, Pods: req.Pods, Tail: msg.Tail, Container: req.Container}
	}
}
