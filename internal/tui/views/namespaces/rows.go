package namespaces

import (
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// tableHeight is the body less the lines above the table: the filter bar
// and the stale banner.
func (m Model) tableHeight() int {
	h := m.bodyH
	if m.filter.Active() {
		h--
	}
	if m.health.Stale() {
		h--
	}
	if m.prompt.on {
		h -= 2 // the refusal and the prompt
	}
	return h
}

// feedFilter hands a key press or a paste to the open filter and refilters
// the rows. consumed is false when the filter is closed or let the key pass.
func (m Model) feedFilter(msg tea.Msg) (Model, tea.Cmd, bool) {
	if !m.filter.IsOpen() {
		return m, nil, false
	}
	var cmd tea.Cmd
	var consumed bool
	m.filter, cmd, consumed = m.filter.Update(msg)
	if !consumed {
		return m, cmd, false
	}
	return m.withRows(), cmd, true
}

// withRows rebuilds the visible rows. The synthetic "<all>" row is kept
// regardless of the filter — it is the user's escape hatch to reset the
// active namespace and is not a "real" namespace they would want to filter
// out by name.
func (m Model) withRows() Model {
	items := m.raw
	if m.filter.Active() {
		items = make([]k8s.Namespace, 0, len(m.raw))
		for _, n := range m.raw {
			if m.filter.Match(n.Name) {
				items = append(items, n)
			}
		}
	}
	m.shown = items
	m.table.SetRows(toRows(items))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	return m
}

// rowName is the raw name behind table row i: allRowName for the <all>
// row, ok false past the rows. The rows hold cleaned text, so nothing
// reads a name back from a cell.
func (m Model) rowName(i int) (string, bool) {
	switch {
	case i == 0:
		return allRowName, true
	case i > 0 && i <= len(m.shown):
		return m.shown[i-1].Name, true
	}
	return "", false
}

// cursorName is rowName of the row under the cursor.
func (m Model) cursorName() (string, bool) {
	return m.rowName(m.table.Cursor())
}

// toRows renders the <all> row, then items, each cell made inert at draw
// time (sanitize.Cell).
func toRows(items []k8s.Namespace) []table.Row {
	rows := make([]table.Row, 0, len(items)+1)
	rows = append(rows, table.Row{allRowName, "", ""})
	for _, n := range items {
		age := ""
		if !n.Age.IsZero() {
			age = n.Age.Format("2006-01-02 15:04")
		}
		rows = append(rows, table.Row{sanitize.Cell(n.Name), sanitize.Cell(string(n.Status)), age})
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
	if m.prompt.on {
		return m.promptView()
	}
	if m.health.Failed() {
		return styles.Warn.Render("namespaces unavailable: " + m.health.Text())
	}
	body := m.table.View()
	above := make([]string, 0, 3)
	if m.filter.Active() {
		above = append(above, m.filter.View())
	}
	if banner := m.health.Banner(); banner != "" {
		above = append(above, banner)
	}
	if len(above) == 0 {
		return body
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(above, body)...)
}
