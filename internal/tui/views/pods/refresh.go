package pods

import (
	"context"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	minTableRows = 5

	// A list of every namespace is the large one, so it gets longer.
	namespaceTimeout     = 5 * time.Second
	allNamespacesTimeout = 15 * time.Second
)

// ResourceVersions a list asks for (M4.4): the first load and each tick
// may be answered from the apiserver's watch cache; r is the user asking
// for what is there now, so it reads consistently.
const (
	rvCache      = "0"
	rvConsistent = ""
)

func listTimeout(namespace string) time.Duration {
	if namespace == "" {
		return allNamespacesTimeout
	}
	return namespaceTimeout
}

// list fetches the pods under the poller's Token.
func (m Model) list(rv string) tea.Cmd {
	c, ns := m.client, m.namespace
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) ([]k8s.Pod, error) {
		return c.ListPods(ctx, ns, k8s.PodsResourceVersion(rv))
	})
}

func (m Model) onTick(msg async.TickMsg) (Model, tea.Cmd) {
	poll, due := m.poll.Due(msg)
	if !due {
		return m, nil
	}
	m.poll = poll
	return m, m.list(rvCache)
}

// refresh is r: a consistent read now, unless a list is already in flight.
func (m Model) refresh() (Model, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	poll, ok := m.poll.Begin()
	if !ok {
		return m, nil
	}
	m.poll = poll
	return m, m.list(rvConsistent)
}

// onList folds a list reply in. Done arms the next tick, and the Cmd it
// returns must reach the program or polling stops.
func (m Model) onList(msg async.Result[[]k8s.Pod]) (Model, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil
	}
	var next tea.Cmd
	m.poll, next = m.poll.Done(msg.Token, msg.Err)
	m.loaded = true
	m.err = msg.Err
	if msg.Err != nil {
		return m, next
	}
	m.raw = msg.Value
	return m.refiltered(), next
}

func (m Model) resized(msg tea.WindowSizeMsg) Model {
	m.bodyH = msg.Height
	m.filter.SetWidth(msg.Width)
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	return m
}

// tableHeight reserves one body line for the filter bar when active so the
// table does not overlap the prompt.
func (m Model) tableHeight() int {
	if m.filter.Active() {
		return m.bodyH - 1
	}
	return m.bodyH
}

func (m Model) openFilter() (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Open()
	return m.refiltered(), cmd
}

// feedFilter hands a key press or a paste to the open filter and refilters
// the rows; anything the filter does not take changes nothing.
func (m Model) feedFilter(msg tea.Msg) (Model, tea.Cmd) {
	f, cmd, consumed := m.filter.Update(msg)
	if !consumed {
		return m, nil
	}
	m.filter = f
	return m.refiltered(), cmd
}

// refiltered rebuilds the rows from raw through the filter. The cursor
// stays on the pod it was on when that pod is still shown: rows move
// between fetches, and a filter change should not jump to another pod.
func (m Model) refiltered() Model {
	prev, _ := m.cursorPod()
	shown := m.raw
	if m.filter.Active() {
		shown = make([]k8s.Pod, 0, len(m.raw))
		for _, p := range m.raw {
			if m.filter.Match(p.Name, p.Namespace, p.Status) {
				shown = append(shown, p)
			}
		}
	}
	m.shown = shown
	m.table.SetRows(toRows(shown, m.namespace == ""))
	m.table.SetHeight(max(m.tableHeight(), minTableRows))
	if i := indexOf(shown, rowKey(prev)); i >= 0 {
		return m.withCursor(i)
	}
	return m.withCursor(m.table.Cursor())
}

// withCursor puts the cursor on row i (clamped) and scrolls it into view.
// The table scrolls only as its cursor moves, and from wherever it was
// scrolled, so a far jump with SetCursor can select a row off screen. The
// cursor walks down from the top of a fresh table instead.
func (m Model) withCursor(i int) Model {
	if len(m.shown) == 0 {
		return m
	}
	i = min(max(i, 0), len(m.shown)-1)
	if i == m.table.Cursor() {
		return m
	}
	t := newTable(m.table.Columns())
	t.SetRows(m.table.Rows())
	t.SetHeight(max(m.tableHeight(), minTableRows))
	t.MoveDown(i)
	m.table = t
	return m
}

func newTable(cols []table.Column) table.Model {
	t := table.New(table.WithColumns(cols), table.WithFocused(true), table.WithHeight(20))
	t.SetStyles(styles.Table())
	styles.FitTable(&t)
	return t
}

// cursorPod is the pod under the cursor, if any.
func (m Model) cursorPod() (k8s.Pod, bool) {
	i := m.table.Cursor()
	if i < 0 || i >= len(m.shown) {
		return k8s.Pod{}, false
	}
	return m.shown[i], true
}

func rowKey(p k8s.Pod) string {
	if p.Name == "" {
		return ""
	}
	return views.RowKey(p.Namespace, p.Name)
}

func indexOf(pods []k8s.Pod, key string) int {
	if key == "" {
		return -1
	}
	for i, p := range pods {
		if rowKey(p) == key {
			return i
		}
	}
	return -1
}
