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

	"github.com/LywwKkA-aD/k4s/internal/k8s"
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
	err       error
	loaded    bool

	watchEnabled bool

	selectKey  key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
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
		m.table.SetHeight(max(msg.Height, minTableRows))
	case tickMsg:
		cmds := []tea.Cmd{tickCmd()}
		if m.watchEnabled && m.client != nil {
			cmds = append(cmds, fetchCmd(m.client, m.namespace))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
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
			m.table.SetRows(toRows(msg.items, m.namespace == ""))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.refreshKey) && m.client != nil:
		return fetchCmd(m.client, m.namespace), true
	case key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0:
		row := m.table.SelectedRow()
		if row == nil {
			return nil, false
		}
		ns, name := serviceCoords(row, m.namespace)
		return func() tea.Msg {
			return views.DescribeRequestMsg{Kind: "service", Namespace: ns, Name: name}
		}, true
	}
	return nil, false
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
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no services")
	}
	return m.table.View()
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
	return []key.Binding{m.selectKey, m.watchKey, m.refreshKey}
}

// Close implements views.View.
func (m Model) Close() error { return nil }
