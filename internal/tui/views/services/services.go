// Package services is the services list view: kubectl-style columns (TYPE,
// CLUSTER-IP, EXTERNAL-IP, PORT(S)) with Enter wired to the describe view.
// 'l' is intentionally absent — services don't produce logs themselves.
package services

import (
	"context"
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

// Model is the services list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	table     table.Model
	raw       []k8s.Service
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
	forwardKey key.Binding
}

// resolvedMsg mirrors the deployments view's same-named local type: the
// result of "for this service, give me its backing pods and the
// container set the picker should offer". Routed back through the root
// model via TailPromptRequestMsg or ContainerPromptRequestMsg depending
// on container count.
type resolvedMsg struct {
	namespace  string
	pods       []string
	containers []string
	err        error
}

type servicesMsg struct {
	items []k8s.Service
	err   error
}

// New constructs a services view.
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
		logsKey: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "tail all pods"),
		),
		forwardKey: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "port-forward"),
		),
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func tableColumns(showNamespace bool) []table.Column {
	cols := make([]table.Column, 0, 7)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	cols = append(cols,
		table.Column{Title: "NAME", Width: 28},
		table.Column{Title: "TYPE", Width: 14},
		table.Column{Title: "CLUSTER-IP", Width: 16},
		table.Column{Title: "EXTERNAL-IP", Width: 18},
		table.Column{Title: "PORT(S)", Width: 22},
		table.Column{Title: "AGE", Width: 8},
	)
	return cols
}

// Init kicks off the first services fetch and the watch ticker.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd()
	}
	return tea.Batch(fetchCmd(m.client, m.namespace), tickCmd())
}

func fetchCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListServices(ctx, namespace)
		return servicesMsg{items: items, err: err}
	}
}

// Update routes refresh / select and forwards navigation to the table.
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
		if key.Matches(msg, m.watchKey) {
			m.watchEnabled = !m.watchEnabled
			return m, nil
		}
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	case servicesMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.raw = msg.items
			m.applyFilter()
		}
		return m, nil
	case resolvedMsg:
		return m, m.dispatchResolved(msg)
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// dispatchResolved is the second half of the 'l' flow: it has the pod
// list and the container set for the highlighted service, and turns
// them into either the tail prompt (1 container) or the container
// picker (>= 2). Mirrors the deployments view so the UX is identical
// whether the user thinks in deployments or services.
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

// resolveServicePodsCmd does the API call in a goroutine and ships the
// result back as resolvedMsg. The hot path stays on the bubbletea loop.
func resolveServicePodsCmd(c *k8s.Client, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		pods, containers, err := c.PodsAndContainersForService(ctx, ns, name)
		if err != nil {
			return resolvedMsg{namespace: ns, err: err}
		}
		return resolvedMsg{namespace: ns, pods: pods, containers: containers}
	}
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
		items = make([]k8s.Service, 0, len(m.raw))
		for _, s := range m.raw {
			if m.filter.Match(s.Name, s.Namespace, s.Type) {
				items = append(items, s)
			}
		}
	}
	m.table.SetRows(toRows(items, m.namespace == ""))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		return m.openFilter(), true
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		return fetchCmd(m.client, m.namespace), true
	case key.Matches(msg, m.selectKey) && m.canActOnRow():
		return m.describeAction(), true
	case key.Matches(msg, m.logsKey) && m.canActOnRow() && m.client != nil:
		return m.logsAction(), true
	case key.Matches(msg, m.forwardKey) && m.canActOnRow():
		return m.forwardAction(), true
	}
	return nil, false
}

// canActOnRow centralises the "has a usable cursor right now" guard
// every row-action needs. Pulled out so handleKey stays under
// golangci-lint's gocyclo threshold.
func (m Model) canActOnRow() bool {
	return m.loaded && m.table.SelectedRow() != nil && len(m.table.Rows()) > 0
}

func (m *Model) openFilter() tea.Cmd {
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Open()
	m.applyFilter()
	return cmd
}

func (m Model) describeAction() tea.Cmd {
	ns, name := serviceCoords(m.table.SelectedRow(), m.namespace)
	return func() tea.Msg {
		return views.DescribeRequestMsg{Kind: "service", Namespace: ns, Name: name}
	}
}

// logsAction mirrors the deployments-view ergonomics so a user who is
// not sure whether to navigate by Service or by Deployment ends up in
// the same place: a logs view tailing every replica.
func (m Model) logsAction() tea.Cmd {
	ns, name := serviceCoords(m.table.SelectedRow(), m.namespace)
	return resolveServicePodsCmd(m.client, ns, name)
}

func (m Model) forwardAction() tea.Cmd {
	ns, name := serviceCoords(m.table.SelectedRow(), m.namespace)
	// Lift the suggested remote port out of the matching raw record
	// so the prompt can offer a sensible default.
	remote := uint16(0)
	for _, s := range m.raw {
		if s.Namespace == ns && s.Name == name {
			remote = firstServicePort(s)
			break
		}
	}
	return func() tea.Msg {
		return views.ForwardRequestMsg{Kind: "service", Namespace: ns, Name: name, RemotePort: remote}
	}
}

// firstServicePort extracts the first declared port from "80/TCP, 443/TCP".
// Returns 0 when nothing parses — the prompt then asks the user.
func firstServicePort(s k8s.Service) uint16 {
	ports := s.Ports
	for i, c := range ports {
		if c == '/' {
			ports = ports[:i]
			break
		}
		if c == ',' {
			ports = ports[:i]
			break
		}
	}
	var p uint16
	for _, c := range ports {
		if c < '0' || c > '9' {
			break
		}
		p = p*10 + uint16(c-'0')
	}
	return p
}

func serviceCoords(row table.Row, scopedNamespace string) (string, string) {
	if scopedNamespace == "" {
		return row[0], row[1]
	}
	return scopedNamespace, row[0]
}

func toRows(items []k8s.Service, showNamespace bool) []table.Row {
	rows := make([]table.Row, 0, len(items))
	for _, s := range items {
		size := 6
		if showNamespace {
			size = 7
		}
		row := make(table.Row, 0, size)
		if showNamespace {
			row = append(row, s.Namespace)
		}
		row = append(row,
			s.Name,
			s.Type,
			s.ClusterIP,
			s.ExternalIP,
			s.Ports,
			k8s.HumanizeDuration(s.Age),
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
		return styles.Hint.Render("loading services…")
	}
	if m.err != nil {
		return styles.Warn.Render("services unavailable: " + m.err.Error())
	}
	body := m.table.View()
	if len(m.table.Rows()) == 0 {
		if m.filter.Active() {
			body = styles.Hint.Render("no services match /" + m.filter.Query())
		} else {
			body = styles.Hint.Render("no services")
		}
	}
	if !m.filter.Active() {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.filter.View(), body)
}

// Title implements views.View. Stable routing name; watch state is in
// KubectlEquivalent's "--watch" suffix.
func (m Model) Title() string { return "services" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	suffix := ""
	if m.watchEnabled {
		suffix = " --watch"
	}
	if m.namespace == "" {
		return "kubectl get services -A" + suffix
	}
	return "kubectl get services -n " + m.namespace + suffix
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.logsKey, m.forwardKey, m.filterKey, m.watchKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View.
func (m Model) Close() error { return nil }
