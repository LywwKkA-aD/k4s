// Package pods is the pod list view (bubbles/table).
package pods

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

// Update handles size changes, refresh keystrokes, async fetches and forwards
// navigation keystrokes to the underlying table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Reserve a few rows for header/footer chrome.
		m.table.SetHeight(max(msg.Height-6, minTableRows))
	case tea.KeyMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil {
			return m, fetchPodsCmd(m.client, m.namespace)
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
			humanizeDuration(p.Age),
		)
		rows = append(rows, row)
	}
	return rows
}

// humanizeDuration mirrors kubectl's "5m", "2h", "3d" output style.
func humanizeDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
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
func (m Model) Help() []key.Binding { return []key.Binding{m.refreshKey} }
