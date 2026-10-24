package views

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// LogsPromptFor is the request the resolved log target t of ref leads to:
// the container picker when its pods run several containers, init
// containers and native sidecars counted, else the tail prompt. A single
// container is named, so a pod with an injected sidecar still streams the
// workload's own. detached marks a target the cursor has left since it
// was asked for.
func LogsPromptFor(ref ResourceRef, t k8s.LogTarget, detached bool) tea.Msg {
	pods := t.PodNames()
	if len(t.Containers) > 1 {
		return ContainerPromptRequestMsg{
			Namespace: ref.Namespace, Pods: pods, Containers: t.Containers, Init: t.Init, NextKind: "logs",
			Target: ref, Logs: t, Detached: detached,
		}
	}
	container := ""
	if len(t.Containers) == 1 {
		container = t.Containers[0]
	}
	return TailPromptRequestMsg{
		Namespace: ref.Namespace, Pods: pods, Container: container,
		Target: ref, Logs: t, Detached: detached,
	}
}
