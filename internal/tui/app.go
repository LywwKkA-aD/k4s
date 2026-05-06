// Package tui hosts the Bubble Tea application: the root model and the
// per-view sub-models composed under it.
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Model is the root Bubble Tea model. Sub-views (pods, logs, exec) will be
// composed in here as they are built.
type Model struct {
	client *k8s.Client
	keys   keys.Map
	width  int
	height int
}

// New constructs the root model. client may be nil — the view handles that.
func New(client *k8s.Client) Model {
	return Model{
		client: client,
		keys:   keys.Default(),
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string {
	title := styles.Title.Render("k4s")
	tagline := styles.Subtitle.Render("a fast TUI for k8s / k3s, with kubectl learning hints")

	var status string
	switch {
	case m.client != nil:
		status = styles.OK.Render("connected → " + m.client.Context)
	default:
		status = styles.Warn.Render("no kubeconfig — set KUBECONFIG or run `make k3s-up`")
	}

	hint := styles.Hint.Render("press q to quit · ? for help (coming soon)")

	body := lipgloss.JoinVertical(lipgloss.Center, title, tagline, "", status, "", hint)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}
