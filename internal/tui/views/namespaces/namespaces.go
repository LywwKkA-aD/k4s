// Package namespaces is the namespace list view; selecting a row emits a
// NamespaceSelectedMsg the root model handles to switch the active namespace.
package namespaces

import (
	"context"
	"sync/atomic"
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
	allRowName    = "<all>"
)

var viewGen atomic.Int64

func nextGen() int64 { return viewGen.Add(1) }

type tickMsg struct {
	gen int64
	t   time.Time
}

// Model is the namespaces list view.
type Model struct {
	client *k8s.Client
	table  table.Model
	raw    []k8s.Namespace
	err    error
	loaded bool
	// busy suppresses overlapping fetches: fetchTimeout == watchInterval,
	// so without the guard requests stack up every tick against a dead API.
	busy  bool
	bodyH int
	gen   int64

	watchEnabled bool
	filter       filter.Model

	selectKey  key.Binding
	watchKey   key.Binding
	refreshKey key.Binding
	filterKey  key.Binding
}

type namespacesMsg struct {
	gen   int64
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
		client:       client,
		table:        t,
		watchEnabled: true,
		filter:       filter.New(),
		gen:          nextGen(),
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
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

func tickCmd(gen int64) tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg{gen: gen, t: t} })
}

// Init kicks off the first fetch and the watch ticker.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd(m.gen)
	}
	return tea.Batch(fetchCmd(m.gen, m.client), tickCmd(m.gen))
}

func fetchCmd(gen int64, c *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		items, err := c.ListNamespaces(ctx)
		return namespacesMsg{gen: gen, items: items, err: err}
	}
}

// Update handles size, refresh, selection and forwards keystrokes to the table.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// msg.Height is already the body height (root model adjusts it).
		m.bodyH = msg.Height
		m.filter.SetWidth(msg.Width)
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
	case tickMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		cmds := []tea.Cmd{tickCmd(m.gen)}
		if m.watchEnabled && m.client != nil && !m.busy {
			m.busy = true
			cmds = append(cmds, fetchCmd(m.gen, m.client))
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
	case namespacesMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.busy = false
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

// handleKey returns (cmd, true) when the key matched a binding.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.filterKey):
		var cmd tea.Cmd
		m.filter, cmd = m.filter.Open()
		m.applyFilter()
		return cmd, true
	case key.Matches(msg, m.watchKey):
		m.watchEnabled = !m.watchEnabled
		return nil, true
	case key.Matches(msg, m.refreshKey) && m.client != nil && !m.busy:
		m.busy = true
		return fetchCmd(m.gen, m.client), true
	case key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0:
		row := m.table.SelectedRow()
		if row == nil {
			return nil, false
		}
		name := row[0]
		if name == allRowName {
			name = ""
		}
		return func() tea.Msg {
			return views.NamespaceSelectedMsg{Namespace: name}
		}, true
	}
	return nil, false
}

func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
}

// applyFilter rebuilds visible rows. The synthetic "<all>" row is kept
// regardless of the filter — it is the user's escape hatch to reset the
// active namespace and is not a "real" namespace they would want to filter
// out by name.
func (m *Model) applyFilter() {
	items := m.raw
	if m.filter.Active() {
		items = make([]k8s.Namespace, 0, len(m.raw))
		for _, n := range m.raw {
			if m.filter.Match(n.Name) {
				items = append(items, n)
			}
		}
	}
	m.table.SetRows(toRows(items))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
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
	body := m.table.View()
	if !m.filter.Active() {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.filter.View(), body)
}

// Title implements views.View. Stable routing name; watch state is in
// KubectlEquivalent's "--watch" suffix.
func (m Model) Title() string { return "namespaces" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if m.watchEnabled {
		return "kubectl get namespaces --watch"
	}
	return "kubectl get namespaces"
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.filterKey, m.watchKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return m.filter.IsOpen() }

// Close implements views.View. No streaming resources held.
func (m Model) Close() error { return nil }
