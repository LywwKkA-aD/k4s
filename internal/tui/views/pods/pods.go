// Package pods is the pod list view (bubbles/table).
package pods

import (
	"context"
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

// Model is the pods list view scoped to a namespace ("" = all).
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

type podsMsg struct {
	pods []k8s.Pod
	err  error
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
		client:    client,
		namespace: namespace,
		table:     t,
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "describe"),
		),
		logsKey: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "logs"),
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
		table.Column{Title: "NAME", Width: 36},
		table.Column{Title: "READY", Width: 7},
		table.Column{Title: "STATUS", Width: 18},
		table.Column{Title: "RESTARTS", Width: 9},
		table.Column{Title: "AGE", Width: 8},
	)
	return cols
}

// Init kicks off the first pod fetch.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchPodsCmd(m.client, m.namespace)
}

func fetchPodsCmd(c *k8s.Client, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		pods, err := c.ListPods(ctx, namespace)
		return podsMsg{pods: pods, err: err}
	}
}

// Update handles size/refresh/select and forwards navigation keys to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// msg.Height is already the body height (root model adjusts it).
		m.table.SetHeight(max(msg.Height, minTableRows))
	case tea.KeyMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil {
			return m, fetchPodsCmd(m.client, m.namespace)
		}
		if key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0 {
			row := m.table.SelectedRow()
			if row != nil {
				ns, name := podCoords(row, m.namespace)
				return m, func() tea.Msg {
					return views.DescribeRequestMsg{Kind: "pod", Namespace: ns, Name: name}
				}
			}
		}
		if key.Matches(msg, m.logsKey) && m.loaded && len(m.table.Rows()) > 0 {
			row := m.table.SelectedRow()
			if row != nil {
				ns, name := podCoords(row, m.namespace)
				return m, func() tea.Msg {
					// Ask the root to prompt for tail; it will dispatch
					// LogsRequestMsg once the user submits a value.
					return views.TailPromptRequestMsg{Namespace: ns, Pods: []string{name}}
				}
			}
		}
	case podsMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.table.SetRows(toRows(msg.pods, m.namespace == ""))
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// podCoords pulls (namespace, name) out of the selected row, accounting for
// whether the table is rendering the NAMESPACE column or not.
func podCoords(row table.Row, scopedNamespace string) (string, string) {
	if scopedNamespace == "" {
		// All namespaces: NAMESPACE | NAME | ...
		return row[0], row[1]
	}
	// Single namespace: NAME | ...
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
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no pods")
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "pods" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if m.namespace == "" {
		return "kubectl get pods -A"
	}
	return "kubectl get pods -n " + m.namespace
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.logsKey, m.refreshKey}
}

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
