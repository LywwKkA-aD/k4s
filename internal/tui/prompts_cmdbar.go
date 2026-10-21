package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// openCommandBar opens the ":" bar in the footer. A status message from an
// earlier action goes: the user has moved on.
func (m Model) openCommandBar() Model {
	m.status = m.status.Clear()
	m.overlay = overlay.NewCommand()
	return m
}

// execCmd runs what was typed into the command bar (command.Parse): a
// view's name; `ns <name>` or `ns -A` to change the namespace; or a list
// with a namespace, `pods demo`, `deploy -n demo`, `svc -A`. What Parse
// refuses goes to the status row.
func (m Model) execCmd(input string) (Model, tea.Cmd) {
	t, err := command.Parse(input)
	if err != nil {
		return m.setError(err.Error())
	}
	m.status = m.status.Clear()
	switch {
	case t.Scoped && t.Name == viewNamespaces:
		return m.switchNamespace(t.Namespace)
	case t.Scoped:
		return m.openIn(t.Name, t.Namespace)
	}
	next := m.switchTo(t.Name)
	return next, next.relayoutCmd()
}

// openIn is a list command with a namespace ("" for -A): the list in
// that namespace, one Esc from the view it was asked from. On the list
// itself it is :ns, which keeps the filter, watch and mode.
func (m Model) openIn(name, ns string) (Model, tea.Cmd) {
	if m.current.Title() == name {
		return m.switchNamespace(ns)
	}
	m = m.pushHistory()
	m = m.enterNamespace(ns)
	m = m.replaceView(name)
	return m, m.relayoutCmd()
}

// switchNamespace is `:ns <name>`, ns "" for `:ns -A`. On a namespaced
// list the same view reloads in the new namespace with its filter, watch
// and mode but not its row, which is not there; from any other view pods
// opens there. command.Parse checked the name as a namespace name only:
// RBAC may forbid reading namespaces, so it is not looked up.
func (m Model) switchNamespace(ns string) (Model, tea.Cmd) {
	view, st := m.current.Title(), stateOf(m.current).WithoutSelection()
	if !isNamespacedList(view) {
		view, st = viewPods, views.State{}
	} else if ns == m.namespace {
		return m.setStatus(status.Info, "already in "+namespaceLabel(ns))
	}
	m = m.pushHistory()
	m = m.enterNamespace(ns)
	m = m.replaceView(view)
	m, cmd := m.restore(st)
	return m, tea.Batch(cmd, m.relayoutCmd())
}

// namespaceLabel names ns for a message: "" is every namespace.
func namespaceLabel(ns string) string {
	if ns == "" {
		return "all namespaces"
	}
	return ns
}

// isNamespacedList reports a view that lists one namespace at a time.
func isNamespacedList(title string) bool {
	switch title {
	case viewPods, viewDeployments, viewServices, viewEvents, viewTop:
		return true
	}
	return false
}
