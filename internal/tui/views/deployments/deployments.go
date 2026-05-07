// Package deployments is the deployments list view: rows show each deployment
// with kubectl-style READY / UP-TO-DATE / AVAILABLE counters. Enter opens
// describe; 'l' resolves the deployment's pods via label selector and asks
// the root model to open the tail prompt — the logs view then streams every
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

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	fetchTimeout = 5 * time.Second
	minTableRows = 5
)

// Model is the deployments list view scoped to a namespace ("" = all).
type Model struct {
	client    *k8s.Client
	namespace string
	table     table.Model
	err       error
	loaded    bool

	selectKey  key.Binding
	logsKey    key.Binding
	refreshKey key.Binding
}

type deploymentsMsg struct {
	items []k8s.Deployment
	err   error
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
		client:    client,
		namespace: namespace,
		table:     t,
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "describe"),
		),
		logsKey: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "tail all replicas"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
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

// Init kicks off the first deployments fetch.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchCmd(m.client, m.namespace)
}

func fetchCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListDeployments(ctx, namespace)
		return deploymentsMsg{items: items, err: err}
	}
}

// Update routes size/refresh/select/logs and forwards navigation to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetHeight(max(msg.Height, minTableRows))
	case tea.KeyMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	case deploymentsMsg:
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
		ns, name := deploymentCoords(row, m.namespace)
		return func() tea.Msg {
			return views.DescribeRequestMsg{Kind: "deployment", Namespace: ns, Name: name}
		}, true
	case key.Matches(msg, m.logsKey) && m.loaded && len(m.table.Rows()) > 0:
		row := m.table.SelectedRow()
		if row == nil {
			return nil, false
		}
		ns, name := deploymentCoords(row, m.namespace)
		return resolvePodsAndPrompt(m.client, ns, name), true
	}
	return nil, false
}

// resolvePodsAndPrompt resolves the deployment's pods via label selector
// (synchronously, in the cmd's goroutine) and emits a TailPromptRequestMsg
// listing every match. The root opens the tail prompt; on submit the logs
// view streams them all in parallel — colour-coded by pod.
func resolvePodsAndPrompt(c *k8s.Client, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		pods, err := c.PodsForDeployment(ctx, ns, name)
		if err != nil || len(pods) == 0 {
			// Silent no-op; the user keeps the deployments view and the
			// table state is preserved. They can press 'l' again later.
			return nil
		}
		return views.TailPromptRequestMsg{Namespace: ns, Pods: pods}
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
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no deployments")
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "deployments" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if m.namespace == "" {
		return "kubectl get deployments -A"
	}
	return "kubectl get deployments -n " + m.namespace
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.logsKey, m.refreshKey}
}

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
