// Package deployments is the deployments list view: rows show each deployment
// with kubectl-style READY / UP-TO-DATE / AVAILABLE counters. Enter opens
// describe; 'l' resolves the deployment's pods + containers and asks the
// root model to open the right prompt — single-container pod templates skip
// straight to the tail prompt, multi-container ones detour through the
// container picker first. Either way the logs view ends up streaming every
// replica through the multi-pod machinery, with a coloured per-pod prefix.
package deployments

import (
	"context"
	"fmt"
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

type tickMsg time.Time

// Model is the deployments list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	table     table.Model
	raw       []k8s.Deployment
	err       error
	loaded    bool
	bodyH     int

	watchEnabled bool
	filter       filter.Model

	selectKey  key.Binding
	logsKey    key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
	filterKey  key.Binding
}

type deploymentsMsg struct {
	items []k8s.Deployment
	err   error
}

// resolvedMsg is the result of "for this deployment, give me its pods and
// container names" — kicked off from the 'l' handler.
type resolvedMsg struct {
	namespace  string
	pods       []string
	containers []string
	err        error
}

// New constructs a deployments view for the given namespace ("" = all).
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
			key.WithHelp("l", "tail all replicas"),
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
	}
}

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 6)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	cols = append(cols,
		table.Column{Title: "NAME", Width: 30},
		table.Column{Title: "READY", Width: 8},
		table.Column{Title: "UP-TO-DATE", Width: 11},
		table.Column{Title: "AVAILABLE", Width: 10},
		table.Column{Title: "AGE", Width: 8},
	)
	return cols
}

// Init kicks off the first deployments fetch and the watch ticker.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd()
	}
	return tea.Batch(fetchCmd(m.client, m.namespace), tickCmd())
}

func tickCmd() tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListDeployments(ctx, namespace)
		return deploymentsMsg{items: items, err: err}
	}
}

// resolvePodsAndContainersCmd issues two API calls in sequence — one for the
// matching pods, one for the container template — and ships the combined
// result back as resolvedMsg.
func resolvePodsAndContainersCmd(c *k8s.Client, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		pods, err := c.PodsForDeployment(ctx, ns, name)
		if err != nil {
			return resolvedMsg{namespace: ns, err: err}
		}
		containers, err := c.ContainersForDeployment(ctx, ns, name)
		if err != nil {
			return resolvedMsg{namespace: ns, pods: pods, err: err}
		}
		return resolvedMsg{namespace: ns, pods: pods, containers: containers}
	}
}

// Update routes size/refresh/select/logs and forwards navigation to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.watchEnabled && m.client != nil {
			cmds = append(cmds, fetchCmd(m.client, m.namespace))
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
	case resolvedMsg:
		return m, m.dispatchResolved(msg)
	case deploymentsMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.raw = msg.items
			m.applyFilter()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
}

func (m *Model) applyFilter() {
	items := m.raw
	if m.filter.Active() {
		items = make([]k8s.Deployment, 0, len(m.raw))
		for _, d := range m.raw {
			if m.filter.Match(d.Name, d.Namespace) {
				items = append(items, d)
			}
		}
	}
	m.table.SetRows(toRows(items, m.namespace == ""))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Open()
		m.applyFilter()
		return cmd, true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		return fetchCmd(m.client, m.namespace), true
	case key.Matches(msg, m.watchKey):
		m.watchEnabled = !m.watchEnabled
		return nil, true
	case key.Matches(msg, m.selectKey) && m.canActOnRow():
		ns, name := deploymentCoords(m.table.SelectedRow(), m.namespace)
		return func() tea.Msg {
			return views.DescribeRequestMsg{Kind: "deployment", Namespace: ns, Name: name}
		}, true
	case key.Matches(msg, m.logsKey) && m.canActOnRow():
		ns, name := deploymentCoords(m.table.SelectedRow(), m.namespace)
		return resolvePodsAndContainersCmd(m.client, ns, name), true
	}
	return nil, false
}

func (m Model) canActOnRow() bool {
	return m.client != nil && m.loaded && len(m.table.Rows()) > 0 && m.table.SelectedRow() != nil
}

// dispatchResolved decides between the fast path (single container → tail
// prompt directly) and the picker detour. Errors and empty results are
// silent no-ops so the table state survives.
func (m Model) dispatchResolved(msg resolvedMsg) tea.Cmd {
	if msg.err != nil || len(msg.pods) == 0 {
		return nil
	}
	if len(msg.containers) <= 1 {
		container := ""
		if len(msg.containers) == 1 {
			container = msg.containers[0]
		}
		return func() tea.Msg {
			return views.TailPromptRequestMsg{Namespace: msg.namespace, Pods: msg.pods, Container: container}
		}
	}
	return func() tea.Msg {
		return views.ContainerPromptRequestMsg{
			Namespace:  msg.namespace,
			Pods:       msg.pods,
			Containers: msg.containers,
			NextKind:   "logs",
		}
	}
}

// deploymentCoords reads (namespace, name) out of the selected row,
// accounting for whether the table is rendering the NAMESPACE column.
func deploymentCoords(row table.Row, scopedNamespace string) (string, string) {
	if scopedNamespace == "" {
		return row[0], row[1]
	}
	return scopedNamespace, row[0]
}

func toRows(items []k8s.Deployment, showNamespace bool) []table.Row {
	rows := make([]table.Row, 0, len(items))
	for _, d := range items {
		size := 5
		if showNamespace {
			size = 6
		}
		row := make(table.Row, 0, size)
		if showNamespace {
			row = append(row, d.Namespace)
		}
		row = append(row,
			d.Name,
			fmt.Sprintf("%d/%d", d.Ready, d.Replicas),
			strconv.Itoa(int(d.UpToDate)),
			strconv.Itoa(int(d.Available)),
			k8s.HumanizeDuration(d.Age),
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
		return styles.Hint.Render("loading deployments…")
	}
	if m.err != nil {
		return styles.Warn.Render("deployments unavailable: " + m.err.Error())
	}
	body := m.table.View()
	if len(m.table.Rows()) == 0 {
		if m.filter.Active() {
			body = styles.Hint.Render("no deployments match /" + m.filter.Query())
		} else {
			body = styles.Hint.Render("no deployments")
		}
	}
	if !m.filter.Active() {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.filter.View(), body)
}

// Title implements views.View. Stable routing name; watch state is in
// KubectlEquivalent's "--watch" suffix.
func (m Model) Title() string { return "deployments" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	suffix := ""
	if m.watchEnabled {
		suffix = " --watch"
	}
	if m.namespace == "" {
		return "kubectl get deployments -A" + suffix
	}
	return "kubectl get deployments -n " + m.namespace + suffix
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.logsKey, m.filterKey, m.watchKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
