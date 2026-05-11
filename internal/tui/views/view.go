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

	// CapturesKeys reports whether the view currently owns the keyboard —
	// for example because a textinput is focused (filter prompt, search
	// prompt). When true the root model must forward all keystrokes to the
	// view instead of running its own global bindings, so that 'q', ':', '?'
	// reach the input rather than being swallowed as navigation.
	CapturesKeys() bool

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

// ContextSelectedMsg is emitted by the contexts view when the user picks a
// kubeconfig context. The root model rebuilds k8s.Client against that
// context, drops navigation history (it referenced resources that may not
// exist in the new cluster), resets the active namespace and lands the
// user on the dashboard.
type ContextSelectedMsg struct {
	Name string
}

// DescribeRequestMsg is emitted when a list view (pods, deployments, ...) wants
// to open a describe screen for the resource currently under the cursor.
type DescribeRequestMsg struct {
	Kind      string
	Namespace string
	Name      string
}

// TailPromptRequestMsg asks the root model to open the tail-lines prompt for
// the given pods. Container is forwarded through to the eventual
// LogsRequestMsg — list views that already know the target container (e.g.
// pods view after a successful container picker round-trip) populate it,
// list views that don't (single-container pods) leave it empty so kubectl
// picks the default container.
type TailPromptRequestMsg struct {
	Namespace string
	Pods      []string
	Container string
}

// LogsRequestMsg is dispatched (by the root, after the tail prompt closes)
// to actually open the streaming logs view.
type LogsRequestMsg struct {
	Namespace string
	Pods      []string
	Tail      int64
	Container string
}

// ContainerPromptRequestMsg asks the root model to open the container picker
// for a multi-container pod (or for a deployment whose pod template has more
// than one container). Once the user picks one, the root dispatches the
// next msg according to NextKind:
//
//	"logs"  → views.TailPromptRequestMsg{Namespace, Pods, Container}
//	"exec"  → views.ExecRequestMsg{Namespace, Pod: Pods[0], Container}
//
// list views with only one container should bypass this message entirely
// and emit the next-kind msg directly.
type ContainerPromptRequestMsg struct {
	Namespace  string
	Pods       []string
	Containers []string
	NextKind   string // "logs" or "exec"
}

// ExecRequestMsg drops the user into a kubectl-exec shell against the named
// pod / container. The root model handles this via tea.ExecProcess so the
// TUI exits cleanly while the shell runs and resumes when it finishes.
type ExecRequestMsg struct {
	Namespace string
	Pod       string
	Container string // optional; "" = kubectl picks default
}

// ForwardRequestMsg is emitted by services/pods/deployments views when the
// user presses 'f' on a row. The root model opens a port-prompt and, on
// confirm, calls forwards.Manager.Register + Start. RemotePort is the
// suggested default — for services it comes from the service's first
// declared port, for pods/deployments the user supplies it.
type ForwardRequestMsg struct {
	Kind       string // "service" | "pod" | "deployment"
	Namespace  string
	Name       string
	RemotePort uint16 // 0 → no hint, user picks both
}
