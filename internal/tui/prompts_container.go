package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// onContainerPromptRequest opens the container picker. A request without
// containers has nothing to pick from and opens nothing.
func (m Model) onContainerPromptRequest(msg views.ContainerPromptRequestMsg) Model {
	if len(msg.Containers) == 0 {
		return m
	}
	m.overlay = overlay.NewContainer(msg)
	return m
}

// dispatchContainerCmd routes the picked container to the follow-up the
// picker was opened for: the tail prompt (keeping the target, so it is
// named there too) or exec in the first pod.
func dispatchContainerCmd(req views.ContainerPromptRequestMsg, container string) tea.Cmd {
	switch req.NextKind {
	case "logs":
		return func() tea.Msg {
			return views.TailPromptRequestMsg{
				Namespace: req.Namespace, Pods: req.Pods, Container: container,
				Target: req.Target, Logs: req.Logs,
			}
		}
	case "exec":
		if len(req.Pods) == 0 {
			return nil
		}
		return func() tea.Msg {
			return views.ExecRequestMsg{Namespace: req.Namespace, Pod: req.Pods[0], Container: container}
		}
	}
	return nil
}
