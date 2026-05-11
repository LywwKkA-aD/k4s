// Package forwards renders the list of port-forwards the user has
// declared, with their live status from forwards.Manager. Enter starts
// or stops the highlighted forward; 'd' removes it from state. The view
// subscribes to manager.Changes() so unattended status flips (a forward
// dying on its own) appear without a manual refresh.
package forwards

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	minTableRows = 5
	// startTimeout caps the wait when the user presses Enter so a flaky
	// cluster does not freeze the UI. ForwardPorts itself respects this
	// context via the manager.
	startTimeout = 10 * time.Second
)

// changedMsg is delivered by the watcher goroutine every time the
// underlying Manager mutates. The Model re-reads List() and re-renders.
type changedMsg struct{}

// Model is the forwards view.
type Model struct {
	manager *forwards.Manager
	table   table.Model
	bodyH   int

	selectKey  key.Binding
	stopKey    key.Binding
	deleteKey  key.Binding
	restartKey key.Binding
}

// New constructs the view. Caller passes the singleton Manager. nil is
// allowed for tests and for the (rare) case where Manager construction
// itself failed at startup.
func New(mgr *forwards.Manager) Model {
	t := table.New(
		table.WithColumns(columns()),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(styles.Table())
	return Model{
		manager: mgr,
		table:   t,
		selectKey: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "start"),
		),
		stopKey: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "stop"),
		),
		restartKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "restart"),
		),
		deleteKey: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "delete"),
		),
	}
}

func columns() []table.Column {
	return []table.Column{
		{Title: "STATUS", Width: 10},
		{Title: "KIND", Width: 11},
		{Title: "NAMESPACE", Width: 16},
		{Title: "NAME", Width: 26},
		{Title: "LOCAL → REMOTE", Width: 18},
		{Title: "DETAIL", Width: 30},
	}
}

// Init subscribes to manager changes so the view refreshes on its own.
func (m Model) Init() tea.Cmd {
	if m.manager == nil {
		return nil
	}
	return tea.Batch(refreshCmd(), watchChangesCmd(m.manager))
}

// watchChangesCmd blocks on the manager's one-shot change channel. When
// it fires, we emit changedMsg and schedule the next wait. This keeps
// the UI in sync with the supervisor goroutines.
func watchChangesCmd(mgr *forwards.Manager) tea.Cmd {
	return func() tea.Msg {
		<-mgr.Changes()
		return changedMsg{}
	}
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg { return changedMsg{} }
}

// Update handles size, key events and Manager change notifications.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.table.SetHeight(max(m.tableHeight(), minTableRows))
		m.rebuildRows()
	case changedMsg:
		m.rebuildRows()
		if m.manager != nil {
			return m, watchChangesCmd(m.manager)
		}
	case tea.KeyMsg:
		if cmd, handled := m.handleKey(msg); handled {
			return m, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.manager == nil {
		return nil, false
	}
	if m.table.SelectedRow() == nil {
		return nil, false
	}
	// The visible table doesn't carry the manager-side ID; it lives in
	// the parallel list returned by snapshot(). idAtCursor reconciles
	// the cursor position with that list.
	id := m.idAtCursor()
	if id == "" {
		return nil, false
	}
	switch {
	case key.Matches(msg, m.selectKey):
		return startCmd(m.manager, id), true
	case key.Matches(msg, m.stopKey):
		_ = m.manager.Stop(id)
		return nil, true
	case key.Matches(msg, m.restartKey):
		return restartCmd(m.manager, id), true
	case key.Matches(msg, m.deleteKey):
		_ = m.manager.Remove(id)
		return nil, true
	}
	return nil, false
}

// idAtCursor returns the manager-side ID for the highlighted row, or "".
// The view stores IDs in a parallel slice so the visible table doesn't
// have to carry them.
func (m Model) idAtCursor() string {
	idx := m.table.Cursor()
	list := m.snapshot()
	if idx < 0 || idx >= len(list) {
		return ""
	}
	return list[idx].Forward.ID
}

func (m Model) snapshot() []forwards.Active {
	if m.manager == nil {
		return nil
	}
	return m.manager.List()
}

func startCmd(mgr *forwards.Manager, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()
		_ = mgr.Start(ctx, id)
		// changedMsg will follow naturally via watchChangesCmd; no need
		// to emit anything here.
		return nil
	}
}

func restartCmd(mgr *forwards.Manager, id string) tea.Cmd {
	return func() tea.Msg {
		_ = mgr.Stop(id)
		ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
		defer cancel()
		_ = mgr.Start(ctx, id)
		return nil
	}
}

func (m *Model) rebuildRows() {
	list := m.snapshot()
	rows := make([]table.Row, 0, len(list))
	for _, a := range list {
		rows = append(rows, rowFor(a))
	}
	m.table.SetRows(rows)
}

func rowFor(a forwards.Active) table.Row {
	detail := ""
	if a.Err != nil {
		detail = a.Err.Error()
	}
	return table.Row{
		a.Status.String(),
		a.Forward.Kind,
		a.Forward.Namespace,
		a.Forward.Name,
		fmt.Sprintf("%d → %d", a.Forward.LocalPort, a.Forward.RemotePort),
		detail,
	}
}

func (m Model) tableHeight() int { return m.bodyH }

// View renders the table or a "no forwards yet" placeholder.
func (m Model) View() string {
	if m.manager == nil {
		return styles.Warn.Render("port-forward subsystem unavailable")
	}
	body := m.table.View()
	if len(m.table.Rows()) == 0 {
		body = lipgloss.JoinVertical(lipgloss.Left,
			styles.Hint.Render("no port-forwards yet"),
			styles.Hint.Render("press 'f' in services / pods / deployments to start one"),
		)
	}
	return body
}

// Title implements views.View.
func (m Model) Title() string { return "forwards" }

// KubectlEquivalent implements views.View. The view itself is not a
// single command, but the highlighted row maps directly to a kubectl
// invocation — surface that when something is selected.
func (m Model) KubectlEquivalent() string {
	if m.manager == nil {
		return ""
	}
	list := m.snapshot()
	idx := m.table.Cursor()
	if idx < 0 || idx >= len(list) {
		return "kubectl port-forward …"
	}
	a := list[idx]
	return fmt.Sprintf("kubectl port-forward -n %s %s/%s %d:%d",
		a.Forward.Namespace, a.Forward.Kind, a.Forward.Name,
		a.Forward.LocalPort, a.Forward.RemotePort)
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.selectKey, m.stopKey, m.restartKey, m.deleteKey}
}

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. Nothing to release at the view level —
// the Manager outlives the view.
func (m Model) Close() error { return nil }
