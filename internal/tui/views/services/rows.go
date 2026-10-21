package services

import (
	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// applyFilter rebuilds the rows from raw through the filter. A non-empty
// keep is the row the cursor stays on when it is still listed; rows move
// between fetches, and an index would land on another service.
func (m *Model) applyFilter(keep string) {
	items := m.raw
	if m.filter.Active() {
		items = make([]k8s.Service, 0, len(m.raw))
		for _, s := range m.raw {
			if m.filter.Match(s.Name, s.Namespace, s.Type) {
				items = append(items, s)
			}
		}
	}
	m.table.SetRows(toRows(items, m.namespace == ""))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	if i := m.rowIndex(keep); i >= 0 {
		m.moveCursor(i)
		return
	}
	// An empty table leaves the cursor at -1, and SetRows does not raise
	// it again when rows come back.
	if m.table.Cursor() < 0 && len(m.table.Rows()) > 0 {
		m.moveCursor(0)
	}
}

// rowIndex is the index of the row keyed key, -1 when it is not listed.
func (m Model) rowIndex(key string) int {
	if key == "" {
		return -1
	}
	for i, row := range m.table.Rows() {
		if views.RowKey(serviceCoords(row, m.namespace)) == key {
			return i
		}
	}
	return -1
}

// moveCursor puts the cursor on row i by moving it, not by SetCursor: the
// table scrolls only on a move, and a far row would stay off screen.
func (m *Model) moveCursor(i int) {
	cur := m.table.Cursor()
	switch {
	case cur < 0:
		m.table.SetCursor(0)
		m.table.MoveDown(i)
	case i > cur:
		m.table.MoveDown(i - cur)
	case i < cur:
		m.table.MoveUp(cur - i)
	}
}

// selectedKey is views.RowKey of the row under the cursor, "" for none.
func (m Model) selectedKey() string {
	row := m.table.SelectedRow()
	if row == nil {
		return ""
	}
	return views.RowKey(serviceCoords(row, m.namespace))
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
		row := make(table.Row, 0, 7)
		if showNamespace {
			row = append(row, s.Namespace)
		}
		rows = append(rows, append(row,
			s.Name,
			s.Type,
			s.ClusterIP,
			s.ExternalIP,
			s.Ports,
			duration.HumanDuration(s.Age),
		))
	}
	return rows
}

// firstServicePort extracts the first declared port from "80/TCP, 443/TCP".
// Returns 0 when nothing parses — the prompt then asks the user.
func firstServicePort(s k8s.Service) uint16 {
	ports := s.Ports
	for i, c := range ports {
		if c == '/' || c == ',' {
			ports = ports[:i]
			break
		}
	}
	var p uint16
	for _, c := range ports {
		if c < '0' || c > '9' {
			break
		}
		p = p*10 + uint16(c-'0')
	}
	return p
}

// View renders the table or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading services…")
	}
	if m.health.Failed() {
		return styles.Warn.Render("services unavailable: " + m.health.Err().Error())
	}
	body := m.table.View()
	if len(m.table.Rows()) == 0 {
		if m.filter.Active() {
			body = styles.Hint.Render("no services match /" + m.filter.Query())
		} else {
			body = styles.Hint.Render("no services")
		}
	}
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
