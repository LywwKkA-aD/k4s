// Package tui hosts the Bubble Tea application: the root model and the
// per-view sub-models composed under it.
package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const statsFetchTimeout = 5 * time.Second

// Model is the root Bubble Tea model. Sub-views (pods, logs, exec) will be
// composed in here as they are built.
type Model struct {
	client *k8s.Client
	keys   keys.Map
	width  int
	height int

	stats        k8s.Stats
	statsErr     error
	statsLoaded  bool
	statsLoading bool
}

// statsMsg is delivered when a Stats() call completes (successfully or not).
type statsMsg struct {
	stats k8s.Stats
	err   error
}

// New constructs the root model. client may be nil — the view handles that.
func New(client *k8s.Client) Model {
	return Model{
		client: client,
		keys:   keys.Default(),
	}
}

func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchStatsCmd(m.client)
}

func fetchStatsCmd(c *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), statsFetchTimeout)
		defer cancel()
		s, err := c.Stats(ctx)
		return statsMsg{stats: s, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.Refresh):
			if m.client != nil && !m.statsLoading {
				m.statsLoading = true
				return m, fetchStatsCmd(m.client)
			}
		}
	case statsMsg:
		m.stats = msg.stats
		m.statsErr = msg.err
		m.statsLoaded = true
		m.statsLoading = false
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

	hint := styles.Hint.Render("press q to quit · r to refresh · ? for help (coming soon)")

	body := lipgloss.JoinVertical(
		lipgloss.Center,
		title,
		tagline,
		"",
		status,
		"",
		m.renderStats(),
		"",
		hint,
	)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) renderStats() string {
	if m.client == nil {
		return ""
	}
	if !m.statsLoaded {
		return styles.Hint.Render("loading cluster stats…")
	}
	if m.statsErr != nil {
		return styles.Warn.Render("stats unavailable: " + m.statsErr.Error())
	}
	line := fmt.Sprintf(
		"namespaces %d  ·  pods %d  ·  deployments %d  ·  services %d",
		m.stats.Namespaces, m.stats.Pods, m.stats.Deployments, m.stats.Services,
	)
	return styles.Stat.Render(line)
}
