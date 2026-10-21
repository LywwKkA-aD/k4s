// Package dashboard is the k4s home screen. It answers three questions in
// order: is the cluster OK (the verdict line), what to run next (the next
// line), and where the problem is (numbered panels). Every panel and every
// row maps to one read-only kubectl command, shown in the footer for the
// focused one; only content k4s derives itself is marked ≈.
//
// The data comes from internal/k8s/dash as plain values: one async.Poller
// drives dash.MakePlan, and each refresh is one dash.Snapshot. Nothing is
// polled while watch is off or the dashboard is not on screen, and an idle
// cluster repaints nothing between snapshots.
package dashboard

import (
	"context"
	"os"
	"strconv"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// fetchTimeout bounds one refresh as a whole. dash bounds each request
	// (5 s in a namespace, 15 s across all) and runs eight at once, so this
	// only fires when the API hangs past every one of them.
	fetchTimeout = 60 * time.Second
	// baseCadence is the fast tier before the first plan says otherwise.
	baseCadence = 5 * time.Second
)

// Model is the dashboard view.
type Model struct {
	client  *k8s.Client
	loadErr error
	home    string
	now     func() time.Time
	fetch   FetchFunc
	session kubectl.Session

	mem  Memory
	poll async.Poller
	// forced is set while a refresh asked for with r is in flight.
	forced bool
	// life ends when the view closes, and a refresh still in flight ends
	// with it: on a slow cluster it would otherwise hold up to eight
	// requests open for a view no longer on screen.
	life context.Context
	stop context.CancelFunc

	lay     layout
	blurred bool
	lastKey time.Time
	// cadence and slowed are the last plan's, for the watch hint; next is
	// when it asks again, backoff included, for the stale banner.
	cadence time.Duration
	slowed  bool
	next    time.Duration

	keys keyMap

	// mo is what moves on the motion clock, frames the clock it is armed
	// on; profile is the terminal's colour profile, and perf times the
	// renders, both for the switches that turn motion off.
	mo      motion
	frames  async.Clock
	profile colorprofile.Profile
	perf    *renderClock
}

// New builds the dashboard for client, or the disconnected screen
// explaining loadErr when client is nil. The first refresh starts on Init.
func New(client *k8s.Client, loadErr error, opts ...Option) Model {
	home, _ := os.UserHomeDir()
	return newModel(client, loadErr, home, opts...)
}

func newModel(client *k8s.Client, loadErr error, home string, opts ...Option) Model {
	cfg := newConfig(opts)
	now := cfg.now()
	mem := cfg.mem.adopt(client, cfg.ns, now)
	fetch := mem.fetch
	if cfg.fetch != nil && client != nil {
		fetch = cfg.fetch
	}
	life, stop := context.WithCancel(context.Background())
	m := Model{
		life:    life,
		stop:    stop,
		client:  client,
		loadErr: loadErr,
		home:    home,
		now:     cfg.now,
		fetch:   fetch,
		session: cfg.session,
		mem:     mem,
		blurred: cfg.blurred,
		lastKey: now,
		cadence: baseCadence,
		next:    baseCadence,
		keys:    newKeyMap(),
		mo:      newMotion(),
		frames:  cfg.frames,
		profile: cfg.profile,
		perf:    &renderClock{},
	}
	m.poll = async.NewPoller(async.New(), async.Config{
		Enabled: fetch != nil && !mem.paused,
		Timeout: fetchTimeout,
		Clock:   cfg.clock,
	})
	if fetch != nil {
		m.poll, _ = m.poll.Begin() // the first refresh; Init sends it
	}
	return m
}

// Init sends the first refresh: everything due after the time away, or
// the whole cluster on a first visit.
func (m Model) Init() tea.Cmd {
	if !m.poll.Busy() {
		return nil
	}
	fetch := m.fetchCmd(m.plan(false))
	if m.mem.loaded || m.still() {
		return fetch
	}
	return tea.Batch(fetch, m.spinCmd())
}

// Memory returns what this dashboard learned, for the next one: see
// Memory.
func (m Model) Memory() Memory { return m.mem }

// Title implements views.View. It is also the routing name.
func (m Model) Title() string { return "dashboard" }

// Command implements views.View: the command of the focused row, else of
// the focused panel; the banner's is the verdict's. Approx (≈) is set only
// where the data layer says k4s summarises.
func (m Model) Command() kubectl.Cmd {
	if m.client == nil {
		return kubectl.Cmd{}
	}
	return toKubectl(m.focusedCmd())
}

func (m Model) focusedCmd() dash.Cmd {
	f := m.mem.focus
	if r, ok := f.selected(m.rows(f.panel)); ok && !r.cmd.IsZero() {
		return r.cmd
	}
	return m.panelCommand(f.panel)
}

// Help implements views.View: the context hints for the footer. Enter is
// listed only where it does something.
func (m Model) Help() []key.Binding {
	out := []key.Binding{m.keys.panel, m.keys.side, m.keys.row}
	if desc := m.enterHint(); desc != "" {
		out = append(out, key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", desc)))
	}
	if m.logsHint() {
		out = append(out, m.keys.logs)
	}
	if _, ok := m.lay.placeOf(dash.PanelTopPods); ok && m.mem.focus.panel == dash.PanelTopPods {
		by := "memory"
		if m.mem.topByMemory {
			by = "cpu"
		}
		out = append(out, key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort by "+by)))
	}
	return append(out, m.keys.refresh, key.NewBinding(key.WithKeys("w"), key.WithHelp("w", m.watchHint())))
}

// HelpMap lists the panels for the help popup: the key that focuses each
// one, its name and its kubectl command, decorated for this session like
// the footer's.
func (m Model) HelpMap() []key.Binding {
	panels := append([]dash.Panel{dash.PanelOverview}, m.lay.visible()...)
	out := make([]key.Binding, 0, len(panels))
	for _, p := range panels {
		k := strconv.Itoa(int(p))
		desc := panelNames[p] + " · " + kubectl.Decorate(toKubectl(m.panelCommand(p)), m.session).Display()
		out = append(out, key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc)))
	}
	return out
}

// watchHint is the cadence for the w hint: "watch 5s", "watch 15s (slow
// API)" while stretched, "paused" with watch off. The compact layouts,
// whose banner has no room for it, keep it in the hints as "w 5s".
func (m Model) watchHint() string {
	if !m.poll.Enabled() {
		return "paused"
	}
	hint := seconds(m.cadence)
	if m.lay.w == 0 || m.lay.kind > kindCompact {
		hint = "watch " + hint
	}
	if m.slowed {
		hint += " (slow API)"
	}
	return hint
}

// CapturesKeys implements views.View: nothing on the dashboard takes typed
// text yet.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View: a refresh in flight is cancelled, and its
// reply is dropped by the next view's Token.
func (m Model) Close() error {
	if m.stop != nil {
		m.stop()
	}
	return nil
}

// Back implements views.BackHandler: Esc steps from a row to its panel and
// from a panel to the banner. On the banner Esc is the root's.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	f, used := m.mem.focus.back()
	if !used {
		return m, nil, false
	}
	m.mem.focus = f
	return m.relayout(), nil, true
}

// shown is which optional panels the cluster has.
func (m Model) shown() shown {
	return shown{k3s: m.mem.snap.K3s.Shown, top: m.mem.snap.Caps.Metrics}
}

// relayout is reflow for callers that already know the cursor's row is
// still there.
func (m Model) relayout() Model {
	m, _ = m.reflow()
	return m
}

// reflow rebuilds the layout when its inputs changed: the size, the
// optional panels, or the accordion's open panel. Focus on a panel this
// layout hides goes back to the banner, and a cursor whose row a new
// snapshot or a shorter panel took away moves to its neighbour; gone names
// that row. Before the first size there is no layout to judge by, and
// remembered focus stays.
func (m Model) reflow() (Model, string) {
	if sh, open := m.shown(), m.mem.focus.expanded; sh != m.lay.sh || open != m.lay.open {
		m.lay = buildLayout(m.lay.w, m.lay.h, sh, open)
	}
	if m.lay.w == 0 || m.lay.h == 0 {
		return m, ""
	}
	if _, ok := m.lay.placeOf(m.mem.focus.panel); !ok && m.mem.focus.panel != dash.PanelOverview {
		m.mem.focus = m.mem.focus.at(dash.PanelOverview)
	}
	var gone string
	m.mem.focus, gone = m.mem.focus.anchor(m.rows(m.mem.focus.panel))
	return m, gone
}

// rows lists the rows the cursor can stand on in panel p, as this layout
// draws them.
func (m Model) rows(p dash.Panel) []row {
	return rowsOf(p, m.mem.snap, m.lay, m.mem.topByMemory)
}

func (m Model) resized(w, h int) Model {
	m.lay = buildLayout(w, h, m.shown(), m.mem.focus.expanded)
	return m.relayout()
}

// seconds renders a cadence as the hints show it: "5s", "60s".
func seconds(d time.Duration) string {
	return strconv.FormatInt(int64(d.Round(time.Second)/time.Second), 10) + "s"
}
