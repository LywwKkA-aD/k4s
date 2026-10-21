package nodes

import (
	"errors"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const kindNode = "node"

// row is the node under the cursor. Without one, cmd is the status that
// says why, so a row key never fails silently.
func (m Model) row() (name string, cmd tea.Cmd, ok bool) {
	switch {
	case m.client == nil:
		return "", statusCmd(status.Warn, "no kubeconfig"), false
	case !m.loaded:
		return "", statusCmd(status.Info, "nodes are still loading"), false
	case m.health.Failed():
		return "", statusCmd(status.Warn, "nodes unavailable, press r"), false
	}
	if name = m.selectedKey(); name == "" {
		return "", statusCmd(status.Warn, "no node selected"), false
	}
	return name, nil, true
}

// describe is Enter: `kubectl describe node <name>`.
func (m Model) describe() tea.Cmd {
	name, cmd, ok := m.row()
	if !ok {
		return cmd
	}
	return func() tea.Msg { return views.DescribeRequestMsg{Kind: kindNode, Name: name} }
}

// Selected implements views.Selection: the node under the cursor, rows of
// a stale list included, so o opens its YAML. There is none before the
// first list lands or when the filter leaves no row.
func (m Model) Selected() (views.ResourceRef, bool) {
	name := m.selectedKey()
	if !m.loaded || name == "" {
		return views.ResourceRef{}, false
	}
	return views.ResourceRef{Kind: kindNode, Name: name}, true
}

// listError is a failed list as the view words it: Error is the text the
// status row and the body show, as stale.Describe words it; Unwrap is the
// API's error, which the poller and the classifier read.
type listError struct {
	err  error
	text string
}

func (e listError) Error() string { return e.text }

func (e listError) Unwrap() error { return e.err }

// explain words err as the status row does: k8s.Classify's text, which
// asks `kubectl auth can-i list nodes` of a refused list.
func explain(err error) error {
	if err == nil {
		return nil
	}
	return listError{err: err, text: stale.Describe(err)}
}

// detail is the API's own text behind a forbidden list, made inert, for
// the body under the explanation; "" otherwise.
func detail(err error) string {
	var le listError
	if !errors.As(err, &le) || !apierrors.IsForbidden(le.err) {
		return ""
	}
	return sanitize.Line(le.err.Error())
}
