package tui

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/events"
	forwardsview "github.com/LywwKkA-aD/k4s/internal/tui/views/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/nodes"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/top"
	yamlview "github.com/LywwKkA-aD/k4s/internal/tui/views/yaml"
)

// maxHistory caps the Esc trail; the oldest entry goes first.
const maxHistory = 50

// historyEntry is a view Esc can return to: which one, in which namespace
// and, for a views.Stateful view, how it looked. The rebuildable views
// (the lists, the dashboard, contexts, forwards) are entries by name;
// describe and YAML by name and the object they show, in ref. Logs push
// their parent on entry but are never entries themselves: a stream is
// not reopened behind the user's back.
type historyEntry struct {
	view      string
	namespace string
	state     views.State
	ref       views.ResourceRef
}

// switchTo records the current view in history then constructs the named view.
func (m Model) switchTo(name string) Model {
	if m.current != nil && m.current.Title() != name {
		m = m.pushHistory()
	}
	return m.replaceView(name)
}

// pushHistory saves the current view, if Esc can bring it back, so that
// it does.
func (m Model) pushHistory() Model {
	e, ok := m.entryOf(m.current)
	if !ok {
		return m
	}
	m.history = pushEntry(m.history, e)
	return m
}

// entryOf is the history entry that brings v back, false for a view that
// cannot be rebuilt.
func (m Model) entryOf(v views.View) (historyEntry, bool) {
	switch v := v.(type) {
	case nil:
		return historyEntry{}, false
	case describe.Model:
		ref, _ := v.Selected()
		return historyEntry{view: viewDescribe, namespace: m.namespace, ref: ref}, true
	case yamlview.Model:
		ref, _ := v.Selected()
		return historyEntry{view: viewYAML, namespace: m.namespace, ref: ref}, true
	}
	if !isRebuildableView(v.Title()) {
		return historyEntry{}, false
	}
	return historyEntry{view: v.Title(), namespace: m.namespace, state: stateOf(v)}, true
}

// pushEntry returns h with e on top in a new backing array, so no other
// copy of the model sees the change. The same view, namespace and object
// as the top entry replaces it, with the newer state; past maxHistory the
// oldest entries go.
func pushEntry(h []historyEntry, e historyEntry) []historyEntry {
	if n := len(h); n > 0 && h[n-1].view == e.view && h[n-1].namespace == e.namespace && h[n-1].ref == e.ref {
		out := slices.Clone(h)
		out[n-1] = e
		return out
	}
	keep := h[max(len(h)+1-maxHistory, 0):]
	out := make([]historyEntry, 0, len(keep)+1)
	out = append(out, keep...)
	return append(out, e)
}

// stateOf is v's State, the zero State for a view that keeps none.
func stateOf(v views.View) views.State {
	if s, ok := v.(views.Stateful); ok {
		return s.State()
	}
	return views.State{}
}

func isRebuildableView(title string) bool {
	switch title {
	case viewDashboard, viewPods, viewNamespaces, viewDeployments, viewServices, viewEvents, viewNodes, viewContexts, viewTop, viewForwards:
		return true
	}
	return false
}

// replaceView builds the named view without touching history.
func (m Model) replaceView(name string) Model {
	if m.build != nil {
		return m.swap(m.build(m, name))
	}
	if v := m.newView(name); v != nil {
		return m.swap(v)
	}
	return m
}

// newView builds the named view in the current namespace; nil for a name
// that is not a rebuildable view.
func (m Model) newView(name string) views.View {
	switch name {
	case viewPods:
		return pods.New(m.client, m.namespace)
	case viewNamespaces:
		return namespaces.New(m.client)
	case viewDeployments:
		return deployments.New(m.client, m.namespace)
	case viewServices:
		return services.New(m.client, m.namespace)
	case viewEvents:
		return events.New(m.client, m.namespace)
	case viewNodes:
		return nodes.New(m.client)
	case viewDashboard:
		return dashboard.New(m.client, m.loadErr)
	case viewContexts:
		return contexts.New(currentContextName(m.client), m.kubeconfigSource())
	case viewTop:
		return top.New(m.client, m.namespace)
	case viewForwards:
		return forwardsview.New(m.forwardMgr)
	}
	return nil
}

// rebuild builds the view of history entry e without touching history.
func (m Model) rebuild(e historyEntry) Model {
	switch e.view {
	case viewDescribe:
		return m.swap(m.describeView(e.ref))
	case viewYAML:
		return m.swap(yamlview.New(m.client, e.ref))
	}
	return m.replaceView(e.view)
}

// describeView describes ref through the session's kubectl.
func (m Model) describeView(ref views.ResourceRef) views.View {
	return describe.New(m.client, describe.Kind(ref.Kind), ref.Namespace, ref.Name, describe.WithKubectl(m.describer))
}

// restore hands st back to a freshly built view (see views.Restorer). The
// zero State is a fresh view and is not handed over.
func (m Model) restore(st views.State) (Model, tea.Cmd) {
	r, ok := m.current.(views.Restorer)
	if !ok || st == (views.State{}) {
		return m, nil
	}
	var cmd tea.Cmd
	m.current, cmd = r.Restore(st)
	return m, cmd
}

// swap closes the outgoing view (so its goroutines / streams are stopped)
// and installs the incoming one.
func (m Model) swap(next views.View) Model {
	if m.current != nil {
		_ = m.current.Close()
	}
	m.current = next
	m.recap = m.recap.add(m.decorate(next.Command()))
	return m
}

// goHome resets history and returns to the dashboard.
func (m Model) goHome() Model {
	m.history = nil
	return m.replaceView(viewDashboard)
}

// back is Esc outside overlays and prompts: the view uses it first (a
// views.BackHandler clears its committed filter), else the previous view
// comes back as it was left.
func (m Model) back() (Model, tea.Cmd) {
	if b, ok := m.current.(views.BackHandler); ok {
		if next, cmd, used := b.Back(); used {
			m.current = next
			return m, cmd
		}
	}
	next, cmd, ok := m.popHistory()
	if !ok {
		return m, nil
	}
	return next, tea.Batch(cmd, next.relayoutCmd())
}

// popHistory restores the previous view, namespace and state; ok is false
// when there is none. The shortened stack is clipped so that nothing can
// append into the popped slot, which an earlier copy of the model holds.
func (m Model) popHistory() (Model, tea.Cmd, bool) {
	if len(m.history) == 0 {
		return m, nil, false
	}
	last := m.history[len(m.history)-1]
	m.history = slices.Clip(m.history[:len(m.history)-1])
	m.namespace = last.namespace
	m = m.rebuild(last)
	m, cmd := m.restore(last.state)
	return m, cmd, true
}
