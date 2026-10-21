package tui

import (
	"slices"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
)

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

// switchTo records the current view in history then constructs the named view.
func (m Model) switchTo(name string) Model {
	if m.current != nil && m.current.Title() != name {
		m = m.pushHistory()
	}
	return m.replaceView(name)
}

// pushHistory saves the current rebuildable view so Esc can return to it.
// Leaf views (describe, logs) have dynamic titles and cannot be rebuilt from
// a title alone, so they are skipped.
func (m Model) pushHistory() Model {
	if m.current == nil {
		return m
	}
	title := m.current.Title()
	if isRebuildableView(title) {
		// Clip first: an earlier copy of the model may share this backing
		// array, and a push must not write into its spare capacity.
		m.history = append(slices.Clip(m.history), historyEntry{
			view:      title,
			namespace: m.namespace,
		})
	}
	return m
}

func isRebuildableView(title string) bool {
	switch title {
	case viewDashboard, viewPods, viewNamespaces, viewDeployments, viewServices, viewContexts, viewTop, viewForwards:
		return true
	}
	return false
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
		return m.swap(dashboard.New(m.client, m.loadErr))
	case viewContexts:
		return m.swap(contexts.New(currentContextName(m.client), m.kubeconfigSource()))
	case viewTop:
		return m.swap(top.New(m.client, m.namespace))
	case viewForwards:
		return m.swap(forwardsview.New(m.forwardMgr))
	}
	return m
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
// The shortened stack is clipped so the next push allocates instead of
// overwriting the popped slot, which an earlier copy of the model still holds.
func (m Model) popHistory() (Model, bool) {
	if len(m.history) == 0 {
		return m, false
	}
	last := m.history[len(m.history)-1]
	m.history = slices.Clip(m.history[:len(m.history)-1])
	m.namespace = last.namespace
	return m.replaceView(last.view), true
}
