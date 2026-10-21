package top

import (
	"cmp"
	"errors"
	"slices"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// newTable builds the right column set for the mode. Recreated on every
// mode switch so the header reflects "PODS" vs "NODES" naturally.
func newTable(md mode, showNamespace bool) table.Model {
	t := table.New(
		table.WithColumns(tableColumns(md, showNamespace)),
		table.WithFocused(true),
		table.WithHeight(20),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)
	return t
}

func tableColumns(md mode, showNamespace bool) []table.Column {
	if md == modeNodes {
		return []table.Column{
			{Title: "NAME", Width: 36},
			{Title: "CPU", Width: 10},
			{Title: "MEMORY", Width: 12},
		}
	}
	cols := make([]table.Column, 0, 4)
	if showNamespace {
		cols = append(cols, table.Column{Title: "NAMESPACE", Width: 18})
	}
	return append(cols,
		table.Column{Title: "NAME", Width: 36},
		table.Column{Title: "CPU", Width: 10},
		table.Column{Title: "MEMORY", Width: 12},
	)
}

func (m Model) withSort(s sortKey) Model {
	m.sort = s
	return m
}

func otherSort(s sortKey) sortKey {
	if s == sortMemory {
		return sortCPU
	}
	return sortMemory
}

// usage is one row: its key, its sort values and its cells.
type usage struct {
	key      string
	cpu, mem int64
	cells    table.Row
	tieBreak string
}

// withRows rebuilds the table from the last answer, busiest first by the
// sort column; equal usage keeps a stable order by name.
func (m Model) withRows() Model {
	items := m.usages()
	slices.SortStableFunc(items, func(a, b usage) int {
		if c := cmp.Compare(m.sortValue(b), m.sortValue(a)); c != 0 {
			return c
		}
		return cmp.Compare(a.tieBreak, b.tieBreak)
	})
	rows := make([]table.Row, 0, len(items))
	keys := make([]string, 0, len(items))
	for _, u := range items {
		rows = append(rows, u.cells)
		keys = append(keys, u.key)
	}
	at, had := m.cursorKey()
	prev := max(m.table.Cursor(), 0)
	m.table.SetRows(rows)
	m.keys = keys
	if len(keys) == 0 {
		return m
	}
	if i := slices.Index(keys, at); had && i >= 0 {
		return m.withCursor(i)
	}
	return m.withCursor(min(prev, len(keys)-1))
}

// cursorKey is the views.RowKey of the row under the cursor, if any.
func (m Model) cursorKey() (string, bool) {
	c := m.table.Cursor()
	if c < 0 || c >= len(m.keys) {
		return "", false
	}
	return m.keys[c], true
}

func (m Model) sortValue(u usage) int64 {
	if m.sort == sortMemory {
		return u.mem
	}
	return u.cpu
}

func (m Model) usages() []usage {
	if m.mode == modeNodes {
		out := make([]usage, 0, len(m.nodes))
		for _, n := range m.nodes {
			out = append(out, usage{
				key: views.RowKey("", n.Name), cpu: n.CPUMilli, mem: n.MemBytes, tieBreak: n.Name,
				cells: table.Row{n.Name, k8s.FormatCPU(n.CPUMilli), k8s.FormatMemory(n.MemBytes)},
			})
		}
		return out
	}
	showNamespace := m.namespace == ""
	out := make([]usage, 0, len(m.pods))
	for _, p := range m.pods {
		cells := make(table.Row, 0, 4)
		if showNamespace {
			cells = append(cells, p.Namespace)
		}
		cells = append(cells, p.Name, k8s.FormatCPU(p.CPUMilli), k8s.FormatMemory(p.MemBytes))
		key := views.RowKey(p.Namespace, p.Name)
		out = append(out, usage{key: key, cpu: p.CPUMilli, mem: p.MemBytes, tieBreak: key, cells: cells})
	}
	return out
}

// installHint is how a cluster without metrics-server gets one.
const installHint = "hint: install metrics-server (kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml)"

// View renders the table or a placeholder. Metrics-server can be missing
// (common in fresh k3s without --metrics-server enabled), so when the
// fetch fails we hint at the cause rather than show a raw error.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading metrics…")
	}
	if m.health.Failed() {
		f := failureOf(m.health.Err())
		if f.missing {
			return styles.Warn.Render(f.text) + "\n\n" + styles.Hint.Render(installHint)
		}
		return styles.Warn.Render(f.text)
	}
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no metrics yet (give metrics-server a few seconds)")
	}
	if banner := m.health.Banner(); banner != "" {
		return lipgloss.JoinVertical(lipgloss.Left, banner, m.table.View())
	}
	return m.table.View()
}

// tableHeight is the body less the stale banner's line.
func (m Model) tableHeight() int {
	if m.health.Stale() {
		return m.bodyH - 1
	}
	return m.bodyH
}

// report is the status for a failed fetch: once when the failure starts or
// changes, and for every fetch the user asked for, so a failing poll does
// not repeat itself every tick.
func (m Model) report(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	if last := m.health.Err(); !m.asked && last != nil && last.Error() == err.Error() {
		return nil
	}
	f := failureOf(err)
	msg := views.StatusMsg{Level: f.level, Text: f.text}
	return func() tea.Msg { return msg }
}

// failure is how a failed fetch reads in the status row and the body.
type failure struct {
	text  string
	level status.Level
	// missing is a cluster without metrics-server, which the body follows
	// with the install hint.
	missing bool
}

// failureOf words err. RBAC refusing metrics is an error with its can-i;
// a metrics API that is absent or not ready, or any other failure, is a
// warning: the rest of k4s works without metrics.
func failureOf(err error) failure {
	switch {
	case k8s.Classify(err).Kind == k8s.KindForbidden:
		return failure{text: stale.Describe(err), level: status.Error}
	case !errors.Is(err, k8s.ErrMetricsUnavailable):
		return failure{text: "metrics unavailable: " + stale.Describe(err), level: status.Warn}
	case apierrors.IsServiceUnavailable(err):
		return failure{text: "metrics unavailable: metrics-server is not ready (503)", level: status.Warn}
	}
	return failure{text: "metrics unavailable: metrics-server is not installed", level: status.Warn, missing: true}
}
