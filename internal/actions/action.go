// Package actions defines the abstraction every user-triggered operation in
// k4s implements. Each Action knows the kubectl command it corresponds to,
// which is the foundation for the "learning hints" feature in the TUI.
package actions

import "context"

// Action is a single user-triggered operation.
//
// Implementations should be cheap to construct; Execute is where the real
// work (and any I/O) lives. KubectlEquivalent must be a copy-pastable command
// the user could run themselves to achieve the same effect — this is what
// the TUI surfaces in the footer / help popup.
type Action interface {
	Name() string
	KubectlEquivalent() string
	Execute(ctx context.Context) error
}

// Static is a non-executing Action whose only job is to advertise its kubectl
// equivalent next to a keystroke. Useful for read-only views.
type Static struct {
	name string
	cmd  string
}

// NewStatic returns a Static action.
func NewStatic(name, kubectlCommand string) Static {
	return Static{name: name, cmd: kubectlCommand}
}

func (s Static) Name() string                    { return s.name }
func (s Static) KubectlEquivalent() string       { return s.cmd }
func (s Static) Execute(_ context.Context) error { return nil }
