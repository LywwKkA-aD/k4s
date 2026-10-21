// Package views defines the contract every k4s screen implements and the
// cross-view messages screens use to talk to the root model.
package views

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
)

// View is the contract every k4s screen implements. It mirrors tea.Model,
// except that Update returns a View (so the root keeps the concrete screen
// without type assertions) and View returns the body as a string: only the
// root model builds the frame.
type View interface {
	// Init returns the screen's first command, run when it is mounted.
	Init() tea.Cmd

	// Update handles a message and returns the next state of the screen.
	Update(msg tea.Msg) (View, tea.Cmd)

	// View renders the body between the root's header and footer.
	View() string

	// Title is rendered in the header.
	Title() string

	// Command is the kubectl command the view's current state maps to. The
	// root shows it in the footer's second row, with --context and
	// --kubeconfig added when a pasted copy would otherwise reach another
	// cluster, and `y` copies it. Build it with kubectl.New or a builder
	// (kubectl.PodLogs, kubectl.SelectorLogs, kubectl.Exec, LogsCommand);
	// never add --context, --kubeconfig or an ApproxMark yourself. Set
	// Approx only where k4s does more or less than the command would. The
	// zero Cmd leaves the row empty.
	Command() kubectl.Cmd

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

// StatusMsg asks the root model to show Text in the footer's status row.
// Errors stay until the next key or status.TTL, whichever is later; every
// other level for status.TTL. A view emits it instead of failing silently,
// for example "deployment web has 0 replicas".
type StatusMsg struct {
	Level status.Level
	Text  string
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

// ResourceRef names the object a request was made for: the row the user
// acted on, which may stand for several pods.
type ResourceRef struct {
	Kind      string // "pod", "deployment" or "service"
	Namespace string
	Name      string
}

// TailPromptRequestMsg asks the root model to open the tail-lines prompt for
// the given pods. Container is forwarded through to the eventual
// LogsRequestMsg — list views that already know the target container (e.g.
// pods view after a successful container picker round-trip) populate it,
// list views that don't (single-container pods) leave it empty so kubectl
// picks the default container.
//
// Target is the row the logs were asked for; the prompt's title names it
// ("tail lines · deployment/web · ns demo"). Left zero, the title names
// the pods instead.
type TailPromptRequestMsg struct {
	Namespace string
	Pods      []string
	Container string
	Target    ResourceRef
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
//	"logs"  → views.TailPromptRequestMsg{Namespace, Pods, Container, Target}
//	"exec"  → views.ExecRequestMsg{Namespace, Pod: Pods[0], Container}
//
// list views with only one container should bypass this message entirely
// and emit the next-kind msg directly. Target names the row in the
// picker's title and is carried on to the tail prompt.
type ContainerPromptRequestMsg struct {
	Namespace  string
	Pods       []string
	Containers []string
	NextKind   string // "logs" or "exec"
	Target     ResourceRef
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
