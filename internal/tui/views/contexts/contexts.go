// Package contexts is the kubeconfig context picker. Reading kubeconfig
// is local I/O so there is no watch / refresh ticker — pressing 'r'
// reloads on demand. Selecting a row emits ContextSelectedMsg; the root
// model rebuilds the active k8s.Client against that context.
package contexts

import (
	"sort"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	minTableRows = 5
	// loadDelay is the artificial small delay before the first fetch — it
	// prevents the "loading" placeholder from flashing on fast disks but
	// also lets the WindowSizeMsg arrive first so the table is sized.
	loadDelay = 50 * time.Millisecond
)

// Model is the contexts list view.
type Model struct {
	table  table.Model
	loaded bool
	err    error

	currentContext string

	selectKey  key.Binding
	refreshKey key.Binding
}

type contextsMsg struct {
	items []k8s.Context
	err   error
}

// New constructs the contexts view. currentContext is what the active
// k8s.Client is currently using — passed in so the picker can render an
// indicator on that row.
func New(currentContext string) Model {
	t := table.New(
		table.WithColumns([]table.Column{
			{Title: "", Width: 2},
			{Title: "NAME", Width: 30},
			{Title: "CLUSTER", Width: 24},
			{Title: "USER", Width: 22},
			{Title: "NAMESPACE", Width: 18},
		}),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	return Model{
		table:          t,
		currentContext: currentContext,
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "switch"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// Init kicks off the first kubeconfig read.
func (m Model) Init() tea.Cmd { return fetchCmd() }

func fetchCmd() tea.Cmd {
	return tea.Tick(loadDelay, func(time.Time) tea.Msg {
		items, err := k8s.ListContexts("")
		return contextsMsg{items: items, err: err}
	})
}

// Update handles size, refresh, and selection.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetHeight(max(msg.Height, minTableRows))
	case contextsMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.table.SetRows(m.toRows(msg.items))
		}
		return m, nil
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.refreshKey):
			return m, fetchCmd()
		case key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0:
			row := m.table.SelectedRow()
			if row == nil {
				return m, nil
			}
			name := row[1] // NAME column
			return m, func() tea.Msg {
				return views.ContextSelectedMsg{Name: name}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// toRows builds the table rows, sorted by name with the current context
// surfaced in the leading marker column.
func (m Model) toRows(items []k8s.Context) []table.Row {
	sorted := make([]k8s.Context, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	rows := make([]table.Row, 0, len(sorted))
	for _, c := range sorted {
		marker := " "
		if c.Name == m.currentContext {
			// Plain rune, no lipgloss styling: the bubbles table truncates
			// cells by rune width without ANSI awareness, so a styled "★"
			// would emit a broken escape sequence and swallow the first
			// letters of the neighbouring NAME column.
			marker = "★"
		}
		rows = append(rows, table.Row{marker, c.Name, c.Cluster, c.AuthInfo, c.Namespace})
	}
	return rows
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if !m.loaded {
		return styles.Hint.Render("loading kubeconfig…")
	}
	if m.err != nil {
		return styles.Warn.Render("contexts unavailable: " + m.err.Error())
	}
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no contexts in kubeconfig")
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "contexts" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string { return "kubectl config get-contexts" }

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
