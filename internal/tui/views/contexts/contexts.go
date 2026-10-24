// Package contexts is the kubeconfig context picker. Reading kubeconfig
// is local I/O so there is no watch / refresh ticker — pressing 'r'
// reloads on demand. Selecting a row emits ContextSelectedMsg; the root
// model rebuilds the active k8s.Client against that context.
package contexts

import (
	"sort"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
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
	table table.Model
	// names are the raw context names behind the rows, in row order. A
	// kubeconfig name is free text and the rows hold it cleaned, so Enter
	// and State read the name here, never back from a cell.
	names  []string
	loaded bool
	err    error
	// pending is the row a Restore asked for, held until the first read.
	pending pendingRow

	currentContext string
	source         k8s.Source

	selectKey  key.Binding
	refreshKey key.Binding
}

type contextsMsg struct {
	items []k8s.Context
	err   error
}

// New constructs the contexts view. currentContext is what the active
// k8s.Client is currently using — passed in so the picker can render an
// indicator on that row. src names the kubeconfig files the session
// loaded, so the picker lists the contexts :ctx can switch to; the zero
// Source is kubectl's own lookup.
func New(currentContext string, src k8s.Source) Model {
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
	styles.FitTable(&t)
	return Model{
		table:          t,
		currentContext: currentContext,
		source:         src,
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
func (m Model) Init() tea.Cmd { return fetchCmd(m.source) }

func fetchCmd(src k8s.Source) tea.Cmd {
	return tea.Tick(loadDelay, func(time.Time) tea.Msg {
		items, err := k8s.ListContextsFrom(src)
		return contextsMsg{items: items, err: err}
	})
}

// Update handles size, refresh, and selection.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.table.SetHeight(max(msg.Height, minTableRows))
	case contextsMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err != nil {
			return m, nil
		}
		sorted := sortedByName(msg.items)
		m.names = contextNames(sorted)
		m.table.SetRows(m.toRows(sorted))
		return m.landPending()
	case tea.KeyPressMsg:
		switch {
		case key.Matches(msg, m.refreshKey):
			return m, fetchCmd(m.source)
		case key.Matches(msg, m.selectKey) && m.loaded && len(m.table.Rows()) > 0:
			name, ok := m.cursorName()
			if !ok {
				return m, nil
			}
			return m, func() tea.Msg {
				return views.ContextSelectedMsg{Name: name}
			}
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// sortedByName returns a copy of items sorted by name.
func sortedByName(items []k8s.Context) []k8s.Context {
	sorted := make([]k8s.Context, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted
}

func contextNames(items []k8s.Context) []string {
	names := make([]string, 0, len(items))
	for _, c := range items {
		names = append(names, c.Name)
	}
	return names
}

// cursorName is the raw name of the context under the cursor.
func (m Model) cursorName() (string, bool) {
	return m.rowName(m.table.Cursor())
}

func (m Model) rowName(i int) (string, bool) {
	if i < 0 || i >= len(m.names) {
		return "", false
	}
	return m.names[i], true
}

// toRows builds the table rows in the order of sorted, with the current
// context surfaced in the leading marker column. The kubeconfig's names
// are made inert at draw time (sanitize.Cell).
func (m Model) toRows(sorted []k8s.Context) []table.Row {
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
		rows = append(rows, table.Row{
			marker,
			sanitize.Cell(c.Name),
			sanitize.Cell(c.Cluster),
			sanitize.Cell(c.AuthInfo),
			sanitize.Cell(c.Namespace),
		})
	}
	return rows
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if !m.loaded {
		return styles.Hint.Render("loading kubeconfig…")
	}
	if m.err != nil {
		return styles.Warn.Render("contexts unavailable: " + sanitize.Line(m.err.Error()))
	}
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no contexts in kubeconfig")
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "contexts" }

// Command implements views.View.
func (m Model) Command() kubectl.Cmd { return kubectl.New("config", "get-contexts") }

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.refreshKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
