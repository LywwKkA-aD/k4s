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

	// Close releases resources held by the view (open log streams, watchers,
	// goroutines). Called by the root model whenever the view is replaced or
	// popped from history. Must be safe to call multiple times.
	Close() error
}

// NamespaceSelectedMsg is emitted by the namespaces view when the user picks
// a namespace; the root model handles it by updating the active namespace and
// switching to the pods view. Empty string means "all namespaces".
type NamespaceSelectedMsg struct {
	Namespace string
}

// DescribeRequestMsg is emitted when a list view (pods, deployments, ...) wants
// to open a describe screen for the resource currently under the cursor.
type DescribeRequestMsg struct {
	Kind      string
	Namespace string
	Name      string
}

// TailPromptRequestMsg asks the root model to open the tail-lines prompt for
// the given pods. The user picks a number, the root then dispatches a
// LogsRequestMsg with that Tail. The two-step dance keeps prompt UI
// (textinput, focus, blink) centralised in the root rather than duplicated
// across every list view that wants to ask "tail how many?".
type TailPromptRequestMsg struct {
	Namespace string
	Pods      []string
}

// LogsRequestMsg is emitted (typically by the root model after the tail
// prompt) to open the streaming logs view. Multiple pods are supported so a
// future deployments view can ask for "tail every replica" in one shot.
type LogsRequestMsg struct {
	Namespace string
	Pods      []string
	Tail      int64
}
