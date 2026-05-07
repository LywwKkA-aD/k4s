// Package top is the kubectl-top equivalent: per-pod CPU/memory pulled from
// the metrics-server REST endpoint, refreshed on the same 5s ticker as the
// other list views. 'n' switches between pod and node mode; both share the
// same ticker / table machinery.
//
// When metrics-server is missing or unreachable the view renders a hint
// (instead of a stack trace) so the user knows to install it and can keep
// the rest of the TUI running.
package top

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	fetchTimeout  = 5 * time.Second
	minTableRows  = 5
	watchInterval = 5 * time.Second
)

// resourceMode tells the view what to fetch on every tick.
type resourceMode int

const (
	modePods resourceMode = iota
	modeNodes
)

type tickMsg time.Time

// Model is the top view.
type Model struct {
	client    *k8s.Client
	namespace string
	mode      resourceMode

	table  table.Model
	err    error
	loaded bool

	watchEnabled bool

	toggleKey  key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
}

type podMetricsMsg struct {
	items []k8s.PodMetric
	err   error
}

type nodeMetricsMsg struct {
	items []k8s.NodeMetric
	err   error
}

// New constructs a top view scoped to the given namespace ("" = all).
func New(client *k8s.Client, namespace string) Model {
	return Model{
		client:       client,
		namespace:    namespace,
		mode:         modePods,
		table:        newTable(modePods, namespace == ""),
		watchEnabled: true,
		toggleKey: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "nodes/pods"),
		),
		watchKey: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "watch"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// newTable builds the right column set for the current mode. Recreated on
// every mode switch so the header reflects "PODS" vs "NODES" naturally.
func newTable(mode resourceMode, showNamespace bool) table.Model {
	cols := tableColumns(mode, showNamespace)
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	return t
}

func tableColumns(mode resourceMode, showNamespace bool) []table.Column {
	if mode == modeNodes {
		return []table.Column{
			{Title: "NAME", Width: 36},
			{Title: "CPU", Width: 10},
			{Title: "MEMORY", Width: 12},
		}
	}
	cols := make([]table.Column, 0, 4)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	cols = append(cols,
		table.Column{Title: "NAME", Width: 36},
		table.Column{Title: "CPU", Width: 10},
		table.Column{Title: "MEMORY", Width: 12},
	)
	return cols
}

// Init kicks off the first fetch for the current mode.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd()
	}
	return tea.Batch(fetchCmd(m), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// fetchCmd dispatches the right metrics call for the current mode.
func fetchCmd(m Model) tea.Cmd {
	if m.mode == modeNodes {
		return fetchNodesCmd(m.client)
	}
	return fetchPodsCmd(m.client, m.namespace)
}

func fetchPodsCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListPodMetrics(ctx, namespace)
		return podMetricsMsg{items: items, err: err}
	}
}

func fetchNodesCmd(c *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListNodeMetrics(ctx)
		return nodeMetricsMsg{items: items, err: err}
	}
}

// Update handles size, watch tick, key bindings, and metrics deliveries.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetHeight(max(msg.Height, minTableRows))
	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.watchEnabled && m.client != nil {
			cmds = append(cmds, fetchCmd(m))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		next, cmd, handled := m.handleKey(msg)
		if handled {
			return next, cmd
		}
	case podMetricsMsg:
		m.loaded = true
		m.err = msg.err
		if msg.err == nil {
			m.table.SetRows(podRows(msg.items, m.namespace == ""))
		}
		return m, nil
	case nodeMetricsMsg:
		m.loaded = true
		m.err = msg.err
		if msg.err == nil {
			m.table.SetRows(nodeRows(msg.items))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// handleKey returns (next-model, cmd, true) when the key matched a binding.
// The model is returned by value because the toggle path mutates several
// fields and we want the call site to keep using the result.
func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.toggleKey):
		if m.mode == modePods {
			m.mode = modeNodes
		} else {
			m.mode = modePods
		}
		m.table = newTable(m.mode, m.namespace == "")
		m.loaded = false
		m.err = nil
		if m.client != nil {
			return m, fetchCmd(m), true
		}
		return m, nil, true
	case key.Matches(msg, m.watchKey):
		m.watchEnabled = !m.watchEnabled
		return m, nil, true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		return m, fetchCmd(m), true
	}
	return m, nil, false
}

// podRows sorts pods by descending CPU so the busiest ones are on top —
// the cluster operator's most common ask is "what's hot right now".
func podRows(items []k8s.PodMetric, showNamespace bool) []table.Row {
	sorted := make([]k8s.PodMetric, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CPUMilli > sorted[j].CPUMilli })

	rows := make([]table.Row, 0, len(sorted))
	for _, p := range sorted {
		row := make(table.Row, 0, 4)
		if showNamespace {
			row = append(row, p.Namespace)
		}
		row = append(row, p.Name, k8s.FormatCPU(p.CPUMilli), k8s.FormatMemory(p.MemBytes))
		rows = append(rows, row)
	}
	return rows
}

func nodeRows(items []k8s.NodeMetric) []table.Row {
	sorted := make([]k8s.NodeMetric, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CPUMilli > sorted[j].CPUMilli })

	rows := make([]table.Row, 0, len(sorted))
	for _, n := range sorted {
		rows = append(rows, table.Row{n.Name, k8s.FormatCPU(n.CPUMilli), k8s.FormatMemory(n.MemBytes)})
	}
	return rows
}

// View renders the table or a placeholder. Metrics-server can be missing
// (common in fresh k3s without --metrics-server enabled), so when the
// fetch fails we hint at the cause rather than show a raw error.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading metrics…")
	}
	if m.err != nil {
		hint := "metrics unavailable: " + m.err.Error()
		if isMetricsServerMissing(m.err) {
			hint += "\n\n" + styles.Hint.Render(
				"hint: install metrics-server (kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml)",
			)
		}
		return styles.Warn.Render(hint)
	}
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no metrics yet (give metrics-server a few seconds)")
	}
	return m.table.View()
}

// isMetricsServerMissing returns true when the error reads like the API
// group simply isn't registered. We cannot type-assert here because the
// error is wrapped via fmt.Errorf("metrics request: %w", ...), so a string
// match is the pragmatic choice.
func isMetricsServerMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "the server could not find") ||
		strings.Contains(msg, "no matches for kind")
}

// Title implements views.View. Stable "top" so the navigation history
// can rebuild the view on Esc. The mode (pods vs nodes) is reflected in
// KubectlEquivalent only.
func (m Model) Title() string { return "top" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if m.mode == modeNodes {
		return "kubectl top nodes"
	}
	if m.namespace == "" {
		return "kubectl top pods -A"
	}
	return "kubectl top pods -n " + m.namespace
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.toggleKey, m.watchKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
