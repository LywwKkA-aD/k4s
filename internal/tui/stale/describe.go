package stale

import (
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// NamespaceHint is the way in when RBAC forbids listing every namespace
// and the kubeconfig context names none to fall back to.
const NamespaceHint = ":ns <name> picks one"

// Describe words err for the status row and error bodies, "" for nil: the
// classified Short and Hint, with NamespaceHint between them when RBAC
// refused a list across every namespace.
func Describe(err error) string {
	d := k8s.Classify(err)
	if d.Kind == k8s.KindForbidden && d.AllNamespaces {
		return d.Short + " · " + NamespaceHint + " · " + d.Hint
	}
	return d.Text()
}

// Fallback reports whether a list view should answer err by listing
// contextNS, the kubeconfig context's namespace, instead: RBAC refused a
// list across every namespace and the context names one. say is the
// status that tells the user, with the can-i that would explain the
// refusal. Without a context namespace there is no fallback, and Describe
// points at :ns instead.
func Fallback(err error, contextNS string) (say tea.Cmd, ok bool) {
	d := k8s.Classify(err)
	if d.Kind != k8s.KindForbidden || !d.AllNamespaces || contextNS == "" {
		return nil, false
	}
	msg := views.StatusMsg{
		Level: status.Warn,
		Text:  "cluster-wide list forbidden; showing " + contextNS + " · " + d.Hint,
	}
	return func() tea.Msg { return msg }, true
}
