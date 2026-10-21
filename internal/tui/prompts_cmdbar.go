package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// allNamespacesArg is `:ns -A`. "all" cannot mean all namespaces: it is a
// valid namespace name.
const allNamespacesArg = "-A"

// openCommandBar opens the ":" bar in the footer. A status message from an
// earlier action goes: the user has moved on.
func (m Model) openCommandBar() Model {
	m.status = m.status.Clear()
	m.overlay = overlay.NewCommand()
	return m
}

// execCmd runs what was typed into the command bar: a view's name, or
// `ns <name>` (`ns -A`) to change the namespace. Only ns takes an
// argument.
func (m Model) execCmd(input string) (Model, tea.Cmd) {
	word, arg := splitCommand(input)
	name, ok := command.Resolve(word)
	if !ok || (arg != "" && name != viewNamespaces) {
		return m.setError(fmt.Sprintf("unknown command: %s", strings.TrimSpace(input)))
	}
	m.status = m.status.Clear()
	if arg != "" {
		return m.switchNamespace(arg)
	}
	next := m.switchTo(name)
	return next, next.relayoutCmd()
}

// splitCommand returns the first word of input and the rest, blanks
// collapsed.
func splitCommand(input string) (word, arg string) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.Join(fields[1:], " ")
}

// switchNamespace is `:ns <name>`. On a namespaced list the same view
// reloads in the new namespace with its filter, watch and mode but not its
// row, which is not there; from any other view pods opens there. The
// name is checked as a namespace name only: RBAC may forbid reading
// namespaces, so it is not looked up.
func (m Model) switchNamespace(arg string) (Model, tea.Cmd) {
	ns := ""
	if arg != allNamespacesArg {
		if err := k8s.ValidateNamespace(arg); err != nil {
			return m.setError(err.Error())
		}
		ns = arg
	}
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
