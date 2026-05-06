// Package views defines the contract every k4s screen implements and the
// cross-view messages screens use to talk to the root model.
package views

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// View is the contract every k4s screen implements.
type View interface {
	tea.Model

	// Title is rendered in the header.
	Title() string

	// KubectlEquivalent is the kubectl command the view's current state maps
	// to. The TUI surfaces it in the footer so users learn the CLI by osmosis.
	KubectlEquivalent() string

	// Help returns the view-specific bindings to render in the footer next to
	// the global ones.
	Help() []key.Binding
}

// NamespaceSelectedMsg is emitted by the namespaces view when the user picks
// a namespace; the root model handles it by updating the active namespace and
// switching to the pods view. Empty string means "all namespaces".
type NamespaceSelectedMsg struct {
	Namespace string
}

// DescribeRequestMsg is emitted when a list view (pods, deployments, ...) wants
// to open a describe screen for the resource currently under the cursor. The
// root model pushes the current view to history and swaps in the describe view.
type DescribeRequestMsg struct {
	Kind      string // "pod", future: "deployment", "service", ...
	Namespace string
	Name      string
}
