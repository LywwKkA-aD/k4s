package events

import (
	"errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// listError is a failed list as the view words it: Error is the text the
// status row, the stale banner and the body show, made inert; Unwrap is
// the API's error, which the poller classifies.
type listError struct {
	err  error
	text string
}

func (e listError) Error() string { return e.text }

func (e listError) Unwrap() error { return e.err }

// explain words err for the list of namespace ("" = all). A forbidden list
// names the scope and the `kubectl auth can-i` that checks it; any other
// error is its own text made inert. It stands in for the M4.3 classifier
// (Classify in internal/k8s), which replaces it once it lands.
func explain(err error, namespace string) error {
	switch {
	case err == nil:
		return nil
	case !apierrors.IsForbidden(err):
		return listError{err: err, text: sanitize.Line(err.Error())}
	case namespace == "":
		return listError{err: err, text: "events forbidden cluster-wide · " + canI("") + " · :ns <name> lists one namespace"}
	}
	return listError{err: err, text: "events forbidden in " + namespace + " · " + canI(namespace)}
}

// detail is the API's own text behind err, made inert, for the body under
// the explanation; "" when explain said it all.
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
