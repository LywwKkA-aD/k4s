package events

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
)

// listError is a failed list as the view words it: Error is the text the
// status row and the body show, as stale.Describe words it; Unwrap is the
// API's error, which the poller and the classifier read.
type listError struct {
	err  error
	text string
}

func (e listError) Error() string { return e.text }

func (e listError) Unwrap() error { return e.err }

// explain words err as the status row does: k8s.Classify's text, with the
// way to one namespace when -A was refused and there is no context
// namespace to fall back to.
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

// canI is the command that checks the list the view makes.
func canI(namespace string) string {
	if namespace == "" {
		return "kubectl auth can-i list events -A"
	}
	return "kubectl auth can-i list events -n " + namespace
}

// canFallBack reports a forbidden -A the view can answer by listing the
// context's namespace: once, and only when the context names one.
func (m Model) canFallBack(err error) bool {
	return err != nil && apierrors.IsForbidden(err) && m.namespace == "" && !m.fellBack &&
		m.client != nil && m.client.ContextNamespace != ""
}
