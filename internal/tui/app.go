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
	"github.com/LywwKkA-aD/k4s/internal/tui/views/namespaces"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

// Model is the root Bubble Tea model. It owns the active namespace, the
// command bar and routes messages to the currently active View.
type Model struct {
	client *k8s.Client
	keys   keys.Map
	width  int
	height int

	namespace string // "" = all
	current   views.View

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
		// Forward so views can resize their internal widgets.
		return m.forwardToView(msg)

	case views.NamespaceSelectedMsg:
		m.namespace = msg.Namespace
		m = m.switchTo("pods")
		return m, m.current.Init()

	case tea.KeyMsg:
		if m.cmdBarOpen {
			return m.handleCmdBar(msg)
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Command):
			m.cmdBarOpen = true
			m.cmdError = ""
			m.cmdBar.Focus()
			return m, textinput.Blink
		case key.Matches(msg, m.keys.Back):
			if m.current.Title() != "dashboard" {
				m = m.switchTo("dashboard")
				return m, m.current.Init()
			}
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

func (m Model) switchTo(name string) Model {
	switch name {
	case "pods":
		m.current = pods.New(m.client, m.namespace)
	case "namespaces":
		m.current = namespaces.New(m.client)
	case "dashboard":
		m.current = dashboard.New(m.client)
	}
	return m
}

// View composes header / body / footer.
func (m Model) View() string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		m.renderHeader(),
		m.current.View(),
		m.renderFooter(),
	)
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
		bindings := []string{"q quit", ": command", "esc back"}
		for _, b := range m.current.Help() {
			h := b.Help()
			if h.Key != "" && h.Desc != "" {
				bindings = append(bindings, h.Key+" "+h.Desc)
			}
		}
		top = styles.Hint.Render(strings.Join(bindings, "  ·  "))
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
