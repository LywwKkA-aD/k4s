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

// fetchCmd runs p against the current snapshot under the poller's Token,
// until the poller's timeout or the view closes, whichever comes first.
func (m Model) fetchCmd(p dash.Plan) tea.Cmd {
	fetch, prev, life := m.fetch, m.mem.snap, m.life
	return async.Run(m.poll.Token(), m.poll.Timeout(), func(ctx context.Context) (refresh, error) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(life, cancel)()
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
		m.mem = m.mem.with(msg.Value.snap)
		m, gone = m.reflow()
		// A suggestion that left was fixed or outranked: the cursor
		// moving on says enough.
		if gone != "" && m.mem.focus.panel != dash.PanelOverview {
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
	m.cadence, m.slowed, m.next = p.Cadence, p.Slowed, p.Next
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
	m.cadence, m.slowed, m.next = p.Cadence, p.Slowed, p.Next
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
	case key.Matches(msg, m.keys.logs):
		return m.onLogs()
	case key.Matches(msg, m.keys.sort) && m.mem.focus.panel == dash.PanelTopPods:
		m.mem.topByMemory = !m.mem.topByMemory
		return m.relayout(), nil
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
	if m.mem.paused {
		m.mem.pausedAt = m.now()
	}
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
		f = f.up(m.rows(f.panel))
	case key.Matches(msg, m.keys.down):
		f = f.down(m.rows(f.panel))
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

// onEnter opens what the focused row stands for: describe for a pod, a
// deployment or a service, and on a next suggestion what it says (logs or
// describe). On the verdict it focuses the panel of the most severe
// finding. Anything else has no k4s view yet, and y copies its command.
func (m Model) onEnter() (views.View, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	f := m.mem.focus
	if sg, ok := m.suggestionAt(); ok {
		return m, m.runSuggestion(sg)
	}
	if to := m.mem.snap.Verdict.Focus; f.panel == dash.PanelOverview && to != dash.PanelOverview {
		return m.jump(to)
	}
	if r, ok := f.selected(m.rows(f.panel)); ok && describable(r) {
		return m, describeCmd(r.target)
	}
	return m, statusCmd(status.Info, noViewText)
}

// onLogs is l: the tail prompt for the pod under the cursor, or for the
// pod a next suggestion reads logs of.
func (m Model) onLogs() (views.View, tea.Cmd) {
	if m.client == nil {
		return m, nil
	}
	if sg, ok := m.suggestionAt(); ok {
		if readsLogs(sg) {
			return m, logsOfSuggestion(sg)
		}
		return m, statusCmd(status.Info, "no logs for this suggestion · y copies the command")
	}
	f := m.mem.focus
	if r, ok := f.selected(m.rows(f.panel)); ok && r.pod() {
		return m, logsCmd(r.target, r.container)
	}
	return m, statusCmd(status.Info, "no logs here: l reads the logs of a pod row")
}

// enterHint is the enter hint for the footer, "" where Enter only says
// there is no view, or that the panel it would go to is hidden.
func (m Model) enterHint() string {
	f := m.mem.focus
	if sg, ok := m.suggestionAt(); ok {
		return suggestionHint(sg)
	}
	if to := m.mem.snap.Verdict.Focus; f.panel == dash.PanelOverview && to != dash.PanelOverview {
		if _, ok := m.lay.placeOf(to); !ok {
			return ""
		}
		return "go to " + panelNames[to]
	}
	if r, ok := f.selected(m.rows(f.panel)); ok && describable(r) {
		return "describe"
	}
	return ""
}

// logsHint reports whether l opens logs where the cursor is.
func (m Model) logsHint() bool {
	if sg, ok := m.suggestionAt(); ok {
		return readsLogs(sg)
	}
	f := m.mem.focus
	r, ok := f.selected(m.rows(f.panel))
	return ok && r.pod()
}

func statusCmd(level status.Level, text string) tea.Cmd {
	msg := views.StatusMsg{Level: level, Text: text}
	return func() tea.Msg { return msg }
}
