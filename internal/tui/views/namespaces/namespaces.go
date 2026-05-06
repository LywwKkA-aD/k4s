// Package namespaces is the namespace list view; selecting a row emits a
// NamespaceSelectedMsg the root model handles to switch the active namespace.
package namespaces

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
	fetchTimeout = 5 * time.Second
	minTableRows = 5
	allRowName   = "<all>"
)

// Model is the namespaces list view.
type Model struct {
	client *k8s.Client
	table  table.Model
	err    error
	loaded bool

	selectKey  key.Binding
	refreshKey key.Binding
}

type namespacesMsg struct {
	items []k8s.Namespace
	err   error
}

// New constructs the namespaces view.
func New(client *k8s.Client) Model {
	cols := []table.Column{
		{Title: "NAME", Width: 32},
		{Title: "STATUS", Width: 12},
		{Title: "AGE", Width: 18},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())

	return Model{
		client: client,
		table:  t,
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// Init kicks off the first fetch.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchCmd(m.client)
}

func fetchCmd(c *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListNamespaces(ctx)
		return namespacesMsg{items: items, err: err}
	}
}

// Update handles size, refresh, selection and forwards keystrokes to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// msg.Height is already the body height (root model adjusts it).
		m.table.SetHeight(max(msg.Height, minTableRows))
	case tea.KeyMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil {
			return m, fetchCmd(m.client)
		}
		if key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0 {
			row := m.table.SelectedRow()
			if row != nil {
				name := row[0]
				if name == allRowName {
					name = ""
				}
				return m, func() tea.Msg {
					return views.NamespaceSelectedMsg{Namespace: name}
				}
			}
		}
	case namespacesMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.table.SetRows(toRows(msg.items))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func toRows(items []k8s.Namespace) []table.Row {
	rows := make([]table.Row, 0, len(items)+1)
	rows = append(rows, table.Row{allRowName, "", ""})
	for _, n := range items {
		age := ""
		if !n.Age.IsZero() {
			age = n.Age.Format("2006-01-02 15:04")
		}
		rows = append(rows, table.Row{n.Name, string(n.Status), age})
	}
	return rows
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading namespaces…")
	}
	if m.err != nil {
		return styles.Warn.Render("namespaces unavailable: " + m.err.Error())
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "namespaces" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string { return "kubectl get namespaces" }

// Help implements views.View.
func (m Model) Help() []key.Binding { return []key.Binding{m.selectKey, m.refreshKey} }
