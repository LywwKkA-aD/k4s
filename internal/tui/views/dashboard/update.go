package dashboard

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// refresh is the reply of one fetch.
type refresh struct {
	snap dash.Snapshot
}

// Update handles the size, the poller's ticks and replies, terminal focus
// and keys.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resized(msg.Width, msg.Height), nil
	case async.TickMsg:
		return m.onTick(msg)
	case async.Result[refresh]:
		return m.onRefresh(msg)
	case tea.FocusMsg:
		return m.onTerminalFocus(false)
	case tea.BlurMsg:
		return m.onTerminalFocus(true)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	}
	return m, nil
}

// plan is the refresh plan for now: force for r.
func (m Model) plan(force bool) dash.Plan {
	now := m.now()
	in := m.mem.snap.PlanInput(now.Sub(m.mem.start), m.poll.Enabled(), force, dash.Focus{
		Active:  true,
		Blurred: m.blurred,
		Idle:    now.Sub(m.lastKey),
		TopPods: m.lay.drawsTopPods(),
	})
	return dash.MakePlan(in)
}

// fetchCmd runs p against the current snapshot under the poller's Token.
func (m Model) fetchCmd(p dash.Plan) tea.Cmd {
	fetch, prev := m.fetch, m.mem.snap
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) (refresh, error) {
		return refresh{snap: fetch(ctx, p, prev)}, nil
	})
}

func (m Model) onTick(msg async.TickMsg) (views.View, tea.Cmd) {
	var ok bool
	if m.poll, ok = m.poll.Due(msg); !ok {
		return m, nil
	}
	p := m.plan(false)
	if len(p.Due) == 0 {
		// A stretched cadence left nothing due on this tick: arm the next
		// one without a fetch.
		return m.settle(p, m.poll.Token(), nil)
	}
	return m, m.fetchCmd(p)
}

func (m Model) onRefresh(msg async.Result[refresh]) (views.View, tea.Cmd) {
	if !m.poll.Accept(msg.Token) {
		return m, nil // another view's reply, or one this view moved past
	}
	m.forced = false
	var note tea.Cmd
	if msg.Err == nil {
		var gone string
		m.mem, gone = m.mem.with(msg.Value.snap)
		m = m.relayout()
		if gone != "" {
			note = statusCmd(status.Info, goneText(gone))
		}
	}
	next, cmd := m.settle(m.plan(false), msg.Token, msg.Err)
	return next, tea.Batch(cmd, note)
}

// settle ends the poll under tok: the next tick is armed at the plan's
// interval, which carries the blur, idle, size and 429 stretches, and
// never when watch is off.
func (m Model) settle(p dash.Plan, tok async.Token, err error) (Model, tea.Cmd) {
	m.cadence, m.slowed = p.Cadence, p.Slowed
	if p.Arm && p.Next > 0 {
		m.poll, _ = m.poll.WithInterval(p.Next) // not armed while busy: nothing to replace
	}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.Done(tok, err)
	return m, cmd
}

// retime applies a changed focus, blur or idle state to the armed tick: a
// shorter interval re-arms it at once, a longer one applies from the next.
func (m Model) retime() (Model, tea.Cmd) {
	p := m.plan(false)
	m.cadence, m.slowed = p.Cadence, p.Slowed
	if !p.Arm || p.Next <= 0 {
		return m, nil
	}
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithInterval(p.Next)
	return m, cmd
}

func (m Model) onTerminalFocus(blurred bool) (views.View, tea.Cmd) {
	if m.blurred == blurred {
		return m, nil
	}
	m.blurred = blurred
	return m.retime()
}

func (m Model) onKey(msg tea.KeyPressMsg) (views.View, tea.Cmd) {
	m.lastKey = m.now()
	m, wake := m.retime() // a key ends the idle stretch
	next, cmd := m.handleKey(msg)
	return next, tea.Batch(wake, cmd)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (views.View, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.refresh):
		return m.onRefreshKey()
	case key.Matches(msg, m.keys.watch):
		return m.onWatchKey()
	case key.Matches(msg, m.keys.enter):
		return m.onEnter()
	}
	if m.client == nil {
		return m, nil
	}
	return m.move(msg)
}

func (m Model) onRefreshKey() (views.View, tea.Cmd) {
	if m.fetch == nil {
		return m, statusCmd(status.Warn, "not connected: nothing to refresh")
	}
	var ok bool
	if m.poll, ok = m.poll.Begin(); !ok {
		return m, statusCmd(status.Info, "refresh already running")
	}
	m.forced = true
	return m, m.fetchCmd(m.plan(true))
}

func (m Model) onWatchKey() (views.View, tea.Cmd) {
	if m.fetch == nil {
		return m, statusCmd(status.Warn, "not connected: nothing to watch")
	}
	m.mem.paused = !m.mem.paused
	var cmd tea.Cmd
	m.poll, cmd = m.poll.WithEnabled(!m.mem.paused)
	return m, cmd
}

// move handles the focus keys: tab and shift+tab through the panels, 0 to
// 7 to jump, the arrows beside and through rows. h and l move nothing: l
// is logs.
func (m Model) move(msg tea.KeyPressMsg) (views.View, tea.Cmd) {
	f := m.mem.focus
	switch {
	case key.Matches(msg, m.keys.next):
		f = f.cycle(m.order(), +1)
	case key.Matches(msg, m.keys.prev):
		f = f.cycle(m.order(), -1)
	case key.Matches(msg, m.keys.left):
		f = f.at(m.lay.neighbour(f.panel, -1))
	case key.Matches(msg, m.keys.right):
		f = f.at(m.lay.neighbour(f.panel, +1))
	case key.Matches(msg, m.keys.up):
		f = f.up(rowsOf(f.panel, m.mem.snap))
	case key.Matches(msg, m.keys.down):
		f = f.down(rowsOf(f.panel, m.mem.snap))
	case key.Matches(msg, m.keys.jump):
		return m.jump(dash.Panel(msg.Code - '0'))
	default:
		return m, nil
	}
	if f.panel == m.mem.focus.panel && f.row == m.mem.focus.row {
		return m, nil
	}
	m.mem.focus = f
	return m.relayout(), nil
}

// order is the tab order: the banner, then every panel on screen.
func (m Model) order() []dash.Panel {
	return append([]dash.Panel{dash.PanelOverview}, m.lay.visible()...)
}

func (m Model) jump(p dash.Panel) (views.View, tea.Cmd) {
	if _, ok := m.lay.placeOf(p); !ok && p != dash.PanelOverview {
		return m, statusCmd(status.Info, hiddenText(p, m.shown()))
	}
	m.mem.focus = m.mem.focus.at(p)
	return m.relayout(), nil
}

// hiddenText says why panel p is not on screen.
func hiddenText(p dash.Panel, sh shown) string {
	name := panelLabel(p)
	switch {
	case p == dash.PanelK3s && !sh.k3s:
		return name + ": not a k3s cluster, or kube-system is not readable"
	case p == dash.PanelTopPods && !sh.top:
		return name + ": no metrics-server"
	}
	return name + " is hidden at this size: widen the terminal"
}

// onEnter opens what the focused row stands for: describe for a pod. On
// the banner it focuses the panel of the most severe finding. Anything
// else has no k4s view yet, and y copies its command.
func (m Model) onEnter() (views.View, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	f := m.mem.focus
	if f.panel == dash.PanelOverview && m.mem.snap.Verdict.Focus != dash.PanelOverview {
		return m.jump(m.mem.snap.Verdict.Focus)
	}
	r, ok := f.selected(rowsOf(f.panel, m.mem.snap))
	if ok && r.target.Kind == "pod" && r.placed {
		req := views.DescribeRequestMsg{Kind: "pod", Namespace: r.target.Namespace, Name: r.target.Name}
		return m, func() tea.Msg { return req }
	}
	return m, statusCmd(status.Info, "no k4s view · y copies the command")
}

// enterHint is the enter hint for the footer, "" where Enter only says
// there is no view.
func (m Model) enterHint() string {
	f := m.mem.focus
	if f.panel == dash.PanelOverview && m.mem.snap.Verdict.Focus != dash.PanelOverview {
		return "go to " + panelNames[m.mem.snap.Verdict.Focus]
	}
	if r, ok := f.selected(rowsOf(f.panel, m.mem.snap)); ok && r.target.Kind == "pod" && r.placed {
		return "describe"
	}
	return ""
}

func statusCmd(level status.Level, text string) tea.Cmd {
	msg := views.StatusMsg{Level: level, Text: text}
	return func() tea.Msg { return msg }
}
