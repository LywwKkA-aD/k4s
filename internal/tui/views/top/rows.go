package top

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
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
	m.table.SetRows(rows)
	m.keys = keys
	return m
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
	if m.err != nil {
		hint := "metrics unavailable: " + m.err.Error()
		if isMetricsServerMissing(m.err) {
			hint += "\n\n" + styles.Hint.Render(
				"hint: install metrics-server (kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml)",
			)
		}
		return styles.Warn.Render(hint)
	}
	if len(m.table.Rows()) == 0 {
		return styles.Hint.Render("no metrics yet (give metrics-server a few seconds)")
	}
	return m.table.View()
}

// report is the status for a failed fetch: once when the failure starts or
// changes, and for every fetch the user asked for, so a failing poll does
// not repeat itself every tick.
func (m Model) report(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	if !m.asked && m.err != nil && m.err.Error() == err.Error() {
		return nil
	}
	msg := views.StatusMsg{Level: status.Warn, Text: "metrics unavailable: " + err.Error()}
	switch {
	case apierrors.IsForbidden(err):
		msg = views.StatusMsg{Level: status.Error, Text: "metrics forbidden: " + err.Error()}
	case isMetricsServerMissing(err):
		msg.Text = "metrics unavailable: metrics-server is not installed"
	}
	return func() tea.Msg { return msg }
}

// isMetricsServerMissing returns true when the error reads like the API
// group simply isn't registered. We cannot type-assert here because the
// error is wrapped via fmt.Errorf("metrics request: %w", ...), so a string
// match is the pragmatic choice.
func isMetricsServerMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "the server could not find") ||
		strings.Contains(msg, "no matches for kind")
}
