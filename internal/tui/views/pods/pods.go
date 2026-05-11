// Package pods is the pod list view (bubbles/table).
package pods

import (
	"context"
	"strconv"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/filter"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	fetchTimeout  = 5 * time.Second
	minTableRows  = 5
	watchInterval = 5 * time.Second
)

// tickMsg is the per-view auto-refresh signal. Per-view typing means a
// stale ticker that fires after the user has navigated away simply gets
// ignored by whatever view is current — no global ticker plumbing needed.
type tickMsg time.Time

// Model is the pods list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	table     table.Model
	raw       []k8s.Pod
	err       error
	loaded    bool
	bodyH     int

	watchEnabled bool
	filter       filter.Model

	selectKey  key.Binding
	logsKey    key.Binding
	execKey    key.Binding
	forwardKey key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
	filterKey  key.Binding
}

type podsMsg struct {
	pods []k8s.Pod
	err  error
}

// containersResolvedMsg is the result of an async ContainersForPod call.
// We can't open a TailPromptRequestMsg directly from the key handler because
// the resolve is an API call — instead the key handler kicks off a Cmd and
// the result lands here as a message we then translate into the right next
// step (single-container fast path or container-prompt detour).
type containersResolvedMsg struct {
	namespace  string
	pod        string
	containers []string
	nextKind   string // "logs" or "exec"
	err        error
}

// New constructs a pods view for the given namespace ("" = all namespaces).
func New(client *k8s.Client, namespace string) Model {
	t := table.New(
		table.WithColumns(tableColumns(namespace == "")),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())

	return Model{
		client:       client,
		namespace:    namespace,
		table:        t,
		watchEnabled: true,
		filter:       filter.New(),
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "describe"),
		),
		logsKey: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
		),
		execKey: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "exec"),
		),
		watchKey: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "watch"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		filterKey: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		forwardKey: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "port-forward"),
		),
	}
}

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 6)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	cols = append(cols,
		table.Column{Title: "NAME", Width: 36},
		table.Column{Title: "READY", Width: 7},
		table.Column{Title: "STATUS", Width: 18},
		table.Column{Title: "RESTARTS", Width: 9},
		table.Column{Title: "AGE", Width: 8},
	)
	return cols
}

// Init kicks off the first pod fetch and the auto-refresh ticker.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd()
	}
	return tea.Batch(fetchPodsCmd(m.client, m.namespace), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchPodsCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		pods, err := c.ListPods(ctx, namespace)
		return podsMsg{pods: pods, err: err}
	}
}

// resolveContainersCmd is fired from the 'l' / 'e' key handlers; it figures
// out the pod's containers off the bubbletea event loop and ships the
// answer back as a containersResolvedMsg for the model to act on.
func resolveContainersCmd(c *k8s.Client, ns, pod, kind string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		containers, err := c.ContainersForPod(ctx, ns, pod)
		return containersResolvedMsg{
			namespace:  ns,
			pod:        pod,
			containers: containers,
			nextKind:   kind,
			err:        err,
		}
	}
}

// Update handles size/refresh/select and forwards navigation keys to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.watchEnabled && m.client != nil {
			cmds = append(cmds, fetchPodsCmd(m.client, m.namespace))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		if m.filter.IsOpen() {
			var cmd tea.Cmd
			var consumed bool
			m.filter, cmd, consumed = m.filter.Update(msg)
			if consumed {
				m.applyFilter()
				return m, cmd
			}
		}
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	case containersResolvedMsg:
		return m, m.dispatchContainers(msg)
	case podsMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.raw = msg.pods
			m.applyFilter()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// tableHeight reserves one body line for the filter bar when active so the
// table does not overlap the prompt.
func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
}

// applyFilter rebuilds the visible rows from the raw cache using the current
// filter query. Called on every podsMsg, every keystroke inside the filter,
// and whenever the filter is opened/closed.
func (m *Model) applyFilter() {
	pods := m.raw
	if m.filter.Active() {
		pods = make([]k8s.Pod, 0, len(m.raw))
		for _, p := range m.raw {
			if m.filter.Match(p.Name, p.Namespace, p.Status) {
				pods = append(pods, p)
			}
		}
	}
	m.table.SetRows(toRows(pods, m.namespace == ""))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
}

// handleKey returns (cmd, true) if the key matched a view binding.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Open()
		m.applyFilter()
		return cmd, true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		return fetchPodsCmd(m.client, m.namespace), true
	case key.Matches(msg, m.watchKey):
		m.watchEnabled = !m.watchEnabled
		return nil, true
	case key.Matches(msg, m.selectKey) && m.canActOnRow():
		ns, name := podCoords(m.table.SelectedRow(), m.namespace)
		return func() tea.Msg {
			return views.DescribeRequestMsg{Kind: "pod", Namespace: ns, Name: name}
		}, true
	case key.Matches(msg, m.logsKey) && m.canActOnRow():
		ns, name := podCoords(m.table.SelectedRow(), m.namespace)
		return resolveContainersCmd(m.client, ns, name, "logs"), true
	case key.Matches(msg, m.execKey) && m.canActOnRow():
		ns, name := podCoords(m.table.SelectedRow(), m.namespace)
		return resolveContainersCmd(m.client, ns, name, "exec"), true
	case key.Matches(msg, m.forwardKey) && m.canActOnRow():
		ns, name := podCoords(m.table.SelectedRow(), m.namespace)
		return func() tea.Msg {
			return views.ForwardRequestMsg{Kind: "pod", Namespace: ns, Name: name}
		}, true
	}
	return nil, false
}

func (m Model) canActOnRow() bool {
	return m.client != nil && m.loaded && len(m.table.Rows()) > 0 && m.table.SelectedRow() != nil
}

// dispatchContainers turns the API result into the next msg: skip the
// container picker for single-container pods, route through the picker
// otherwise. Errors are silent — the user keeps the table state.
func (m Model) dispatchContainers(msg containersResolvedMsg) tea.Cmd {
	if msg.err != nil || len(msg.containers) == 0 {
		return nil
	}
	if len(msg.containers) == 1 {
		container := msg.containers[0]
		switch msg.nextKind {
		case "logs":
			return func() tea.Msg {
				return views.TailPromptRequestMsg{Namespace: msg.namespace, Pods: []string{msg.pod}, Container: container}
			}
		case "exec":
			return func() tea.Msg {
				return views.ExecRequestMsg{Namespace: msg.namespace, Pod: msg.pod, Container: container}
			}
		}
		return nil
	}
	return func() tea.Msg {
		return views.ContainerPromptRequestMsg{
			Namespace:  msg.namespace,
			Pods:       []string{msg.pod},
			Containers: msg.containers,
			NextKind:   msg.nextKind,
		}
	}
}

// podCoords pulls (namespace, name) out of the selected row, accounting for
// whether the table is rendering the NAMESPACE column or not.
func podCoords(row table.Row, scopedNamespace string) (string, string) {
	if scopedNamespace == "" {
		return row[0], row[1]
	}
	return scopedNamespace, row[0]
}

func toRows(pods []k8s.Pod, showNamespace bool) []table.Row {
	rows := make([]table.Row, 0, len(pods))
	for _, p := range pods {
		size := 5
		if showNamespace {
			size = 6
		}
		row := make(table.Row, 0, size)
		if showNamespace {
			row = append(row, p.Namespace)
		}
		row = append(row,
			p.Name,
			p.Ready,
			p.Status,
			strconv.Itoa(int(p.Restarts)),
			k8s.HumanizeDuration(p.Age),
		)
		rows = append(rows, row)
	}
	return rows
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading pods…")
	}
	if m.err != nil {
		return styles.Warn.Render("pods unavailable: " + m.err.Error())
	}

	body := m.table.View()
	if len(m.table.Rows()) == 0 {
		if m.filter.Active() {
			body = styles.Hint.Render("no pods match /" + m.filter.Query())
		} else {
			body = styles.Hint.Render("no pods")
		}
	}
	if !m.filter.Active() {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.filter.View(), body)
}

// Title implements views.View. Routing depends on Title returning the
// stable view name; the watch state is signalled via the "--watch" suffix
// in KubectlEquivalent.
func (m Model) Title() string { return "pods" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	suffix := ""
	if m.watchEnabled {
		suffix = " --watch"
	}
	if m.namespace == "" {
		return "kubectl get pods -A" + suffix
	}
	return "kubectl get pods -n " + m.namespace + suffix
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.logsKey, m.execKey, m.forwardKey, m.filterKey, m.watchKey, m.refreshKey}
}

// CapturesKeys implements views.View. Returns true while the filter prompt
// is focused so that 'q' / ':' / '?' / etc. reach the input as text.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
