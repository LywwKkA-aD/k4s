// Package tui hosts the Bubble Tea application: the root model, view router,
// command bar and chrome (header + footer).
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

// View names used both for routing and for history entries.
const (
	viewDashboard  = "dashboard"
	viewPods       = "pods"
	viewNamespaces = "namespaces"
)

// historyEntry captures the navigation snapshot we restore on Esc.
//
// Note: only "rebuildable" views (dashboard / pods / namespaces) end up here.
// Leaf views like describe push their parent on entry but never end up in
// history themselves, so popHistory always lands on something we can rebuild
// from {view, namespace}.
type historyEntry struct {
	view      string
	namespace string
}

// Model is the root Bubble Tea model.
type Model struct {
	client *k8s.Client
	keys   keys.Map
	width  int
	height int

	namespace string // "" = all
	current   views.View
	history   []historyEntry

	cmdBar     textinput.Model
	cmdBarOpen bool
	cmdError   string
}

// New constructs the root model with the dashboard as the landing screen.
func New(client *k8s.Client) Model {
	ti := textinput.New()
	ti.Prompt = ": "
	ti.Placeholder = "pods, ns, dashboard"
	ti.CharLimit = 64
	ti.Width = 40

	return Model{
		client:  client,
		keys:    keys.Default(),
		cmdBar:  ti,
		current: dashboard.New(client),
	}
}

// Init starts the active view.
func (m Model) Init() tea.Cmd {
	return m.current.Init()
}

// Update is the message router.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m.forwardToView(msg)

	case views.NamespaceSelectedMsg:
		m.history = append(m.history, historyEntry{
			view:      m.current.Title(),
			namespace: m.namespace,
		})
		m.namespace = msg.Namespace
		m = m.replaceView(viewPods)
		return m, m.current.Init()

	case views.DescribeRequestMsg:
		m.history = append(m.history, historyEntry{
			view:      m.current.Title(),
			namespace: m.namespace,
		})
		m.current = describe.New(m.client, describe.Kind(msg.Kind), msg.Namespace, msg.Name)
		return m, m.current.Init()

	case tea.KeyMsg:
		// ctrl+c is the always-quit escape hatch — even inside the cmd bar.
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.cmdBarOpen {
			return m.handleCmdBar(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			if m.current.Title() == viewDashboard {
				return m, tea.Quit
			}
			next := m.goHome()
			return next, next.current.Init()
		case key.Matches(msg, m.keys.Command):
			m.cmdBarOpen = true
			m.cmdError = ""
			m.cmdBar.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Back):
			if next, ok := m.popHistory(); ok {
				return next, next.current.Init()
			}
			return m, nil
		}
	}

	return m.forwardToView(msg)
}

func (m Model) forwardToView(msg tea.Msg) (tea.Model, tea.Cmd) {
	upd, cmd := m.current.Update(msg)
	if v, ok := upd.(views.View); ok {
		m.current = v
	}
	return m, cmd
}

func (m Model) handleCmdBar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.cmdBarOpen = false
		m.cmdBar.Reset()
		m.cmdBar.Blur()
		return m, nil
	case tea.KeyEnter:
		input := m.cmdBar.Value()
		m.cmdBarOpen = false
		m.cmdBar.Reset()
		m.cmdBar.Blur()
		return m.execCmd(input)
	}
	var cmd tea.Cmd
	m.cmdBar, cmd = m.cmdBar.Update(msg)
	return m, cmd
}

func (m Model) execCmd(input string) (Model, tea.Cmd) {
	name, ok := command.Resolve(input)
	if !ok {
		m.cmdError = fmt.Sprintf("unknown command: %s", strings.TrimSpace(input))
		return m, nil
	}
	m.cmdError = ""
	next := m.switchTo(name)
	return next, next.current.Init()
}

// switchTo records the current view in history then constructs the named view.
func (m Model) switchTo(name string) Model {
	if m.current != nil && m.current.Title() != name {
		m.history = append(m.history, historyEntry{
			view:      m.current.Title(),
			namespace: m.namespace,
		})
	}
	return m.replaceView(name)
}

// replaceView builds the named view without touching history.
func (m Model) replaceView(name string) Model {
	switch name {
	case viewPods:
		m.current = pods.New(m.client, m.namespace)
	case viewNamespaces:
		m.current = namespaces.New(m.client)
	case viewDashboard:
		m.current = dashboard.New(m.client)
	}
	return m
}

// goHome resets history and returns to the dashboard.
func (m Model) goHome() Model {
	m.history = nil
	return m.replaceView(viewDashboard)
}

// popHistory restores the previous view + namespace; returns false if empty.
func (m Model) popHistory() (Model, bool) {
	if len(m.history) == 0 {
		return m, false
	}
	last := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	m.namespace = last.namespace
	return m.replaceView(last.view), true
}

// View composes header / body / footer with the footer pinned near the
// bottom (one line of breathing room — see styles.Footer.PaddingBottom).
func (m Model) View() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	hh := lipgloss.Height(header)
	fh := lipgloss.Height(footer)

	bodyHeight := m.height - hh - fh
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	bodyWidth := max(m.width, 0)

	body := lipgloss.Place(
		bodyWidth,
		bodyHeight,
		lipgloss.Left,
		lipgloss.Top,
		m.current.View(),
	)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) renderHeader() string {
	parts := []string{styles.Title.Render("k4s")}
	if m.client != nil {
		parts = append(parts, styles.Hint.Render("ctx: "+m.client.Context))
	}
	ns := m.namespace
	if ns == "" {
		ns = "ALL"
	}
	parts = append(parts,
		styles.Hint.Render("ns: "+ns),
		styles.Hint.Render("view: "+m.current.Title()),
	)

	w := max(m.width-2, 0)
	return styles.Header.Width(w).Render(strings.Join(parts, "  ·  "))
}

func (m Model) renderFooter() string {
	w := max(m.width-2, 0)

	if m.cmdBarOpen {
		return styles.Footer.Width(w).Render(m.cmdBar.View())
	}

	var top string
	if m.cmdError != "" {
		top = styles.Warn.Render(m.cmdError)
	} else {
		top = styles.Hint.Render(strings.Join(m.footerBindings(), "  ·  "))
	}

	bottom := ""
	if eq := m.current.KubectlEquivalent(); eq != "" {
		bottom = styles.KubectlHint.Render("≈ " + eq)
	}

	if bottom == "" {
		return styles.Footer.Width(w).Render(top)
	}
	return styles.Footer.Width(w).Render(lipgloss.JoinVertical(lipgloss.Left, top, bottom))
}

func (m Model) footerBindings() []string {
	quitLabel := "q quit"
	if m.current.Title() != viewDashboard {
		quitLabel = "q home"
	}
	bindings := []string{quitLabel, "^c quit", "esc back", ": command"}
	for _, b := range m.current.Help() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			bindings = append(bindings, h.Key+" "+h.Desc)
		}
	}
	return bindings
}
