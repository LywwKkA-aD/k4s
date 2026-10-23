// Package forwards renders the list of port-forwards the user has
// declared, with their live status from forwards.Manager. Enter starts the
// highlighted forward, s stops it, r restarts it, d removes it from state
// and ctrl+r re-reads the list. The view subscribes to manager.Changes()
// so unattended status flips (a forward dying on its own) appear without a
// manual refresh.
package forwards

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const minTableRows = 5

// changedMsg is delivered by the watcher once the Manager has mutated. It
// carries the Token of the view that armed the watcher.
type changedMsg struct{ tok async.Token }

// ageTickMsg redraws the rows so the age of a tunnel error keeps counting.
// tok tells the view's current tick chain from one it replaced.
type ageTickMsg struct{ tok async.Token }

// ageEvery is how often a tunnel error's age is redrawn: its first two
// minutes count in seconds.
const ageEvery = time.Second

// Option changes how New builds the view.
type Option func(*Model)

// WithClock makes the view read the time from now, which ages the tunnel
// errors it shows.
func WithClock(now func() time.Time) Option {
	return func(m *Model) { m.now = now }
}

// Model is the forwards view.
type Model struct {
	manager *forwards.Manager
	table   table.Model
	// shown are the forwards behind the table rows, in row order. The
	// cursor, the keys and Command read them, never a fresh List, so a key
	// acts on the row the user sees.
	shown []forwards.Active
	// busy holds the ids whose operation is running. A key on such a row is
	// ignored until the result lands: a repeat would run the operation twice
	// and report a delete the user wanted as a gone forward. Rebuilt, never
	// mutated, so copies of the Model do not share it.
	busy map[string]struct{}
	// changes was taken in New before the first List, so the wait Init
	// arms sees every change since.
	changes <-chan struct{}
	// notice is what the Manager had to say about loading the saved
	// forwards, put on the status line by Init.
	notice string
	// tok tells this view's watcher from one a replaced view left behind.
	tok async.Token
	// ageTok names the live chain of age ticks; starting a chain moves it
	// on, so at most one chain keeps ticking.
	ageTok async.Token
	now    func() time.Time
	bodyH  int
	done   chan struct{}
	keys   keyMap
}

// New constructs the view. Caller passes the singleton Manager. nil is
// allowed for tests and for the (rare) case where Manager construction
// itself failed at startup.
func New(mgr *forwards.Manager, opts ...Option) Model {
	t := table.New(
		table.WithColumns(columns()),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	t.SetStyles(styles.Table())
	styles.FitTable(&t)
	m := Model{
		manager: mgr,
		table:   t,
		tok:     async.New(),
		ageTok:  async.New(),
		now:     time.Now,
		done:    make(chan struct{}),
		keys:    newKeyMap(),
	}
	for _, opt := range opts {
		opt(&m)
	}
	if mgr == nil {
		return m
	}
	m.changes = mgr.Changes()
	m.notice = sanitize.Line(mgr.TakeNotice())
	m.show(mgr.List())
	return m
}

// Init waits for the Manager's next change, ages the tunnel errors shown,
// and says what went wrong loading the saved forwards if anything did.
func (m Model) Init() tea.Cmd {
	if m.manager == nil {
		return nil
	}
	var notice tea.Cmd
	if m.notice != "" {
		notice = statusCmd(views.StatusMsg{Level: status.Warn, Text: m.notice})
	}
	return tea.Batch(m.wait(m.changes), m.ageAfter(m.ageTok), notice)
}

// wait blocks on the manager's one-shot change channel until it fires or
// the view is closed, which keeps the UI in sync with the supervisor
// goroutines without leaking a watcher past Close.
func (m Model) wait(changes <-chan struct{}) tea.Cmd {
	done, tok := m.done, m.tok
	return func() tea.Msg {
		select {
		case <-done:
			return nil
		case <-changes:
			return changedMsg{tok: tok}
		}
	}
}

// Update handles size, key events, Manager changes and finished operations.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.bodyH = msg.Height
		m.table.SetHeight(max(m.bodyH, minTableRows))
		return m, nil
	case changedMsg:
		if !m.tok.Accept(msg.tok) || m.manager == nil {
			return m, nil
		}
		// Changes before List: a change landing in between fires the
		// channel the next wait blocks on.
		changes := m.manager.Changes()
		m.show(m.manager.List())
		var age tea.Cmd
		m, age = m.restartAging()
		return m, tea.Batch(m.wait(changes), age)
	case ageTickMsg:
		if !m.ageTok.Accept(msg.tok) || m.manager == nil {
			return m, nil
		}
		m.refresh()
		return m, m.ageAfter(m.ageTok)
	case forwardOpResultMsg:
		m.busy = without(m.busy, msg.fwd.ID)
		m.refresh()
		var age tea.Cmd
		m, age = m.restartAging()
		return m, tea.Batch(statusCmd(msg.status()), age)
	case tea.KeyPressMsg:
		if next, cmd, handled := m.handleKey(msg); handled {
			return next, cmd
		}
	}
	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

// restartAging starts a new chain of age ticks if a row shows a tunnel
// error. The chain it replaces stops at its next tick.
func (m Model) restartAging() (Model, tea.Cmd) {
	m.ageTok = m.ageTok.Next()
	return m, m.ageAfter(m.ageTok)
}

// ageAfter schedules the next redraw of tok's chain while a row shows the
// age of a tunnel error.
func (m Model) ageAfter(tok async.Token) tea.Cmd {
	for _, a := range m.shown {
		if !a.TunnelErrAt.IsZero() {
			return tea.Tick(ageEvery, func(time.Time) tea.Msg { return ageTickMsg{tok: tok} })
		}
	}
	return nil
}

// refresh re-reads the list. The watcher stays armed on its own channel.
func (m *Model) refresh() {
	if m.manager != nil {
		m.show(m.manager.List())
	}
}

// View renders the table or a "no forwards yet" placeholder.
func (m Model) View() string {
	if m.manager == nil {
		return styles.Warn.Render(unavailable)
	}
	if len(m.shown) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			styles.Hint.Render("no port-forwards yet"),
			styles.Hint.Render("press 'f' in services / pods / deployments to start one"),
		)
	}
	return m.table.View()
}

// Title implements views.View.
func (m Model) Title() string { return "forwards" }

// Command implements views.View: the highlighted row as the kubectl
// port-forward it mirrors; with no row there is nothing to show.
func (m Model) Command() kubectl.Cmd {
	f, ok := m.selected()
	if !ok {
		return kubectl.Cmd{}
	}
	return portForwardCmd(f.Forward)
}

// Help implements views.View.
func (m Model) Help() []key.Binding { return m.keys.help() }

// CapturesKeys implements views.View.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. Signals any in-flight watcher goroutine
// to return so the view can be garbage-collected.
func (m Model) Close() error {
	select {
	case <-m.done:
	default:
		close(m.done)
	}
	return nil
}
