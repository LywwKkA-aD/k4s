// Package dashboard is the landing screen — cluster-wide counters, refreshable.
package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const fetchTimeout = 5 * time.Second

// Model is the dashboard view.
type Model struct {
	client *k8s.Client
	stats  k8s.Stats
	err    error
	loaded bool
	busy   bool

	refreshKey key.Binding
}

type statsMsg struct {
	stats k8s.Stats
	err   error
}

// New constructs the dashboard view.
func New(client *k8s.Client) Model {
	return Model{
		client: client,
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// Init kicks off the first stats fetch.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchStatsCmd(m.client)
}

func fetchStatsCmd(c *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		s, err := c.Stats(ctx)
		return statsMsg{stats: s, err: err}
	}
}

// Update handles refresh keystrokes and stats deliveries.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil && !m.busy {
			m.busy = true
			return m, fetchStatsCmd(m.client)
		}
	case statsMsg:
		m.stats = msg.stats
		m.err = msg.err
		m.loaded = true
		m.busy = false
	}
	return m, nil
}

// View renders the dashboard body (header / footer come from the root).
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig — set KUBECONFIG or run `make k3s-up`")
	}
	if !m.loaded {
		return styles.Hint.Render("loading cluster stats…")
	}
	if m.err != nil {
		return styles.Warn.Render("stats unavailable: " + m.err.Error())
	}
	line := fmt.Sprintf(
		"namespaces %d  ·  pods %d  ·  deployments %d  ·  services %d",
		m.stats.Namespaces, m.stats.Pods, m.stats.Deployments, m.stats.Services,
	)
	return styles.Stat.Render(line)
}

// Title implements views.View.
func (m Model) Title() string { return "dashboard" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string { return "kubectl get all -A" }

// Help implements views.View.
func (m Model) Help() []key.Binding { return []key.Binding{m.refreshKey} }
