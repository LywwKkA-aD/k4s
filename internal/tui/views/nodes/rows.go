package nodes

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/table"
	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/util/duration"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Column widths. STATUS holds "NotReady,SchedulingDisabled", VERSION a
// managed cluster's "v1.30.4-eks-a737599", OS/ARCH "windows/amd64"; NAME
// and ROLES share what the screen has left.
const (
	statusWidth  = 27
	ageWidth     = 6
	versionWidth = 20
	ipWidth      = 15
	osArchWidth  = 13
	taintsWidth  = 6
	fixedWidth   = statusWidth + ageWidth + versionWidth + ipWidth + osArchWidth + taintsWidth
	columns      = 8

	minNameWidth  = 16
	maxNameWidth  = 63
	minRolesWidth = 13
	maxRolesWidth = 26

	// defaultWidth lays the table out before the first WindowSizeMsg.
	defaultWidth = 120
)

// layout fits the columns into width; narrower screens keep the minimums
// and wrap.
func layout(width int) []table.Column {
	if width <= 0 {
		width = defaultWidth
	}
	rest := width - columns*styles.Table().Cell.GetHorizontalFrameSize() - fixedWidth
	roles := min(max(rest/2, minRolesWidth), maxRolesWidth)
	name := min(max(rest-roles, minNameWidth), maxNameWidth)
	return []table.Column{
		{Title: "NAME", Width: name},
		{Title: "STATUS", Width: statusWidth},
		{Title: "ROLES", Width: roles},
		{Title: "AGE", Width: ageWidth},
		{Title: "VERSION", Width: versionWidth},
		{Title: "INTERNAL-IP", Width: ipWidth},
		{Title: "OS/ARCH", Width: osArchWidth},
		{Title: "TAINTS", Width: taintsWidth},
	}
}

// toItems makes every cell of ns inert once: a kubelet reports its own
// version, addresses and platform.
func toItems(ns []k8s.Node) []item {
	out := make([]item, 0, len(ns))
	for _, n := range ns {
		out = append(out, item{name: n.Name, cells: table.Row{
			cell(n.Name),
			cell(n.Status),
			cell(n.Roles),
			duration.HumanDuration(n.Age),
			cell(n.Version),
			cell(n.InternalIP),
			cell(n.OSArch),
			strconv.Itoa(n.Taints),
		}})
	}
	return out
}

// blanks turns the whitespace a table cell cannot hold into spaces: a
// tab, which the cell would widen after measuring it, cannot push the
// next column out.
var blanks = strings.NewReplacer("\n", " ", "\t", " ")

// cell is s on one inert line.
func cell(s string) string {
	return sanitize.Line(blanks.Replace(s))
}

// applyFilter rebuilds the rows from items through the filter. A
// non-empty keep is the row the cursor stays on when it is still listed.
func (m *Model) applyFilter(keep string) {
	shown := make([]string, 0, len(m.items))
	rows := make([]table.Row, 0, len(m.items))
	for _, it := range m.items {
		if m.filter.Active() && !m.filter.Match(it.cells...) {
			continue
		}
		shown = append(shown, it.name)
		rows = append(rows, it.cells)
	}
	m.shown = shown
	m.table.SetRows(rows)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	if i := m.rowIndex(keep); i >= 0 {
		m.moveCursor(i)
		return
	}
	// An empty table leaves the cursor at -1, and SetRows does not raise
	// it again when rows come back.
	if m.table.Cursor() < 0 && len(rows) > 0 {
		m.moveCursor(0)
	}
}

// rowIndex is the index of the node named key, -1 when it is not listed.
func (m Model) rowIndex(key string) int {
	if key == "" {
		return -1
	}
	for i, name := range m.shown {
		if name == key {
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

// selectedKey is the name of the node under the cursor, "" for none: a
// node is cluster-scoped, so its name is its views.RowKey.
func (m Model) selectedKey() string {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.shown) {
		return ""
	}
	return m.shown[i]
}

// View renders the table or a placeholder, under the filter bar and the
// stale banner when there are any.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	above := make([]string, 0, 2)
	if m.filter.Active() {
		above = append(above, m.filter.View())
	}
	if banner := m.health.Banner(); banner != "" {
		above = append(above, banner)
	}
	if len(above) == 0 {
		return m.body()
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(above, m.body())...)
}

func (m Model) body() string {
	switch {
	case !m.loaded:
		return styles.Hint.Render("loading nodes…")
	case m.health.Failed():
		text := styles.Warn.Render("nodes unavailable: " + m.health.Text())
		if d := detail(m.health.Err()); d != "" {
			text = lipgloss.JoinVertical(lipgloss.Left, text, styles.Hint.Render(d))
		}
		return text
	case len(m.shown) > 0:
		return m.table.View()
	case m.filter.Active():
		return styles.Hint.Render("no nodes match /" + m.filter.Query())
	}
	return styles.Hint.Render("no nodes")
}
