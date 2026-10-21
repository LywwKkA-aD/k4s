package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// stubView stands in for a list view that keeps its state, restores it and
// clears its committed filter on Esc. restored records the State it was
// given back; it is a value, so every copy keeps its own.
type stubView struct {
	title    string
	state    views.State
	restored *views.State
}

func (s stubView) Init() tea.Cmd                        { return nil }
func (s stubView) Update(tea.Msg) (views.View, tea.Cmd) { return s, nil }
func (s stubView) View() string                         { return s.title + " filter=" + s.state.Filter }
func (s stubView) Title() string                        { return s.title }
func (s stubView) Command() kubectl.Cmd                 { return kubectl.New("get", s.title) }
func (s stubView) Help() []key.Binding                  { return nil }
func (s stubView) CapturesKeys() bool                   { return false }
func (s stubView) Close() error                         { return nil }
func (s stubView) State() views.State                   { return s.state }
func (s stubView) Restore(st views.State) (views.View, tea.Cmd) {
	s.state = st
	s.restored = &st
	return s, nil
}

func (s stubView) Back() (views.View, tea.Cmd, bool) {
	if s.state.Filter == "" {
		return s, nil, false
	}
	s.state.Filter = ""
	return s, nil, true
}

// stubbed returns a model whose views are stubs of their name.
func stubbed() Model {
	m := New(nil, nil)
	m.build = func(_ Model, name string) views.View { return stubView{title: name} }
	m.current = stubView{title: viewDashboard}
	return m
}

func (m Model) withState(st views.State) Model {
	v, ok := m.current.(stubView)
	if !ok {
		panic("current view is not a stub")
	}
	v.state = st
	m.current = v
	return m
}

func press(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// TestEscRestoresTheStateItLeft: a view's State goes into history when the
// user moves on, and comes back through Restore on Esc.
func TestEscRestoresTheStateItLeft(t *testing.T) {
	t.Parallel()

	st := views.State{Filter: "pay", Selected: "shop/api", Cursor: 2, Paused: true}
	m := stubbed().switchTo(viewDeployments).withState(st)
	m = m.switchTo(viewServices)
	if got := m.history[len(m.history)-1]; got.view != viewDeployments || got.state != st {
		t.Fatalf("history top = %+v, want deployments with %+v", got, st)
	}

	m = press(t, m, tuitest.Esc())
	v, ok := m.current.(stubView)
	if !ok || v.title != viewDeployments || v.restored == nil || *v.restored != st {
		t.Fatalf("after Esc: %+v, want deployments restored with %+v", m.current, st)
	}
}

// TestZeroStateIsNotRestored: a view that kept nothing comes back fresh.
func TestZeroStateIsNotRestored(t *testing.T) {
	t.Parallel()

	m := stubbed().switchTo(viewPods).switchTo(viewServices)
	m = press(t, m, tuitest.Esc())
	if v := m.current.(stubView); v.title != viewPods || v.restored != nil {
		t.Fatalf("after Esc: %+v, want fresh pods", v)
	}
}

// TestEscAsksTheViewFirst: a view with a committed filter clears it and
// stays; the next Esc goes back.
func TestEscAsksTheViewFirst(t *testing.T) {
	t.Parallel()

	m := stubbed().switchTo(viewPods).withState(views.State{Filter: "web"})
	m = press(t, m, tuitest.Esc())
	if v := m.current.(stubView); v.title != viewPods || v.state.Filter != "" {
		t.Fatalf("first Esc: %+v, want pods without its filter", v)
	}
	if len(m.history) != 1 {
		t.Fatalf("history = %+v, the first Esc must not pop", m.history)
	}
	m = press(t, m, tuitest.Esc())
	if v := m.current.(stubView); v.title != viewDashboard {
		t.Fatalf("second Esc: %+v, want the dashboard", v)
	}
}

// TestHistoryIsCapped: alternating two views a hundred times leaves at
// most fifty entries, the oldest dropped.
func TestHistoryIsCapped(t *testing.T) {
	t.Parallel()

	m := New(nil, nil)
	for i := range 100 {
		name := viewPods
		if i%2 == 1 {
			name = viewServices
		}
		m = m.switchTo(name)
	}
	if len(m.history) > maxHistory {
		t.Fatalf("history has %d entries, want at most %d", len(m.history), maxHistory)
	}
	if m.history[0].view == viewDashboard {
		t.Fatal("the oldest entry should have been dropped")
	}
}

// TestHistoryHasNoConsecutiveDuplicates: coming back to the same view in
// the same namespace replaces the top entry, with the newer state.
func TestHistoryHasNoConsecutiveDuplicates(t *testing.T) {
	t.Parallel()

	m := stubbed().switchTo(viewPods).withState(views.State{Filter: "a"})
	m = m.pushHistory()
	m = m.withState(views.State{Filter: "b"})
	m = m.pushHistory()
	if len(m.history) != 2 {
		t.Fatalf("history = %+v, want dashboard and one pods entry", m.history)
	}
	if got := m.history[1].state.Filter; got != "b" {
		t.Fatalf("pods entry filter = %q, want the newer b", got)
	}

	m.namespace = "shop"
	m = m.pushHistory()
	if len(m.history) != 3 {
		t.Fatalf("pods in another namespace is another entry: %+v", m.history)
	}
}

// TestHistoryCopiesNeverShareEntries: pushing on one copy of the model,
// at the cap or below, never changes another copy's history.
func TestHistoryCopiesNeverShareEntries(t *testing.T) {
	t.Parallel()

	m := stubbed()
	for i := range maxHistory + 3 {
		name := viewPods
		if i%2 == 1 {
			name = viewServices
		}
		m = m.switchTo(name)
	}
	snapshot := m
	want := strings.Join(viewsOf(snapshot.history), ",")

	a := snapshot.withState(views.State{Filter: "a"}).switchTo(viewTop)
	b := snapshot.switchTo(viewNamespaces)
	a, _, _ = a.popHistory()
	if got := strings.Join(viewsOf(snapshot.history), ","); got != want {
		t.Fatalf("snapshot history changed:\n got %s\nwant %s", got, want)
	}
	if top := b.history[len(b.history)-1]; top.state.Filter != "" {
		t.Fatalf("b sees a's state: %+v", top)
	}
	_ = a
}

func viewsOf(h []historyEntry) []string {
	out := make([]string, 0, len(h))
	for _, e := range h {
		out = append(out, e.view+"/"+e.namespace+"/"+e.state.Filter)
	}
	return out
}

// TestNamespaceCommandKeepsTheFilter: `:ns` reloads a list in the new
// namespace with its filter and watch, but not its row; from a view that
// lists no namespace it opens fresh pods.
func TestNamespaceCommandKeepsTheFilter(t *testing.T) {
	t.Parallel()

	st := views.State{Filter: "web", Selected: "demo/web-1", Cursor: 3, Paused: true}
	m := stubbed().switchTo(viewDeployments).withState(st)
	next, _ := m.execCmd("ns shop")
	v := next.current.(stubView)
	want := views.State{Filter: "web", Paused: true}
	if v.title != viewDeployments || next.namespace != "shop" || v.restored == nil || *v.restored != want {
		t.Fatalf(":ns shop on deployments = %+v in %q, want deployments restored with %+v", v, next.namespace, want)
	}
	if top := next.history[len(next.history)-1]; top.state != st || top.namespace != "" {
		t.Fatalf("history top = %+v, want the old namespace with its full state", top)
	}

	next, _ = stubbed().withState(st).execCmd("ns shop")
	if v := next.current.(stubView); v.title != viewPods || v.restored != nil {
		t.Fatalf(":ns shop on the dashboard = %+v, want fresh pods", v)
	}
}

// TestContextSwitchDropsSavedStates: a new cluster starts without history.
func TestContextSwitchDropsSavedStates(t *testing.T) {
	t.Parallel()

	m := stubbed().switchTo(viewPods).withState(views.State{Filter: "web"}).switchTo(viewServices)
	next, _ := m.Update(views.ContextSelectedMsg{Name: testContextBeta})
	if h := next.(Model).history; len(h) != 0 {
		t.Fatalf("history after :ctx = %+v", h)
	}
}
