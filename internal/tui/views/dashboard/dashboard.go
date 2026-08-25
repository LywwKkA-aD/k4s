// Package dashboard is the landing screen — cluster-wide counters, refreshable.
package dashboard

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	fetchTimeout  = 5 * time.Second
	watchInterval = 5 * time.Second
)

var viewGen atomic.Int64

func nextGen() int64 { return viewGen.Add(1) }

type tickMsg struct {
	gen int64
	t   time.Time
}

// Model is the dashboard view.
type Model struct {
	client *k8s.Client
	stats  k8s.Stats
	err    error
	loaded bool
	busy   bool
	gen    int64

	watchEnabled bool

	refreshKey key.Binding
	watchKey   key.Binding
}

type statsMsg struct {
	gen   int64
	stats k8s.Stats
	err   error
}

// New constructs the dashboard view.
func New(client *k8s.Client) Model {
	return Model{
		client:       client,
		watchEnabled: true,
		gen:          nextGen(),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		watchKey: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "watch"),
		),
	}
}

func tickCmd(gen int64) tea.Cmd {
	return tea.Tick(watchInterval, func(t time.Time) tea.Msg { return tickMsg{gen: gen, t: t} })
}

// Init kicks off the first stats fetch and the watch ticker.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return tickCmd(m.gen)
	}
	return tea.Batch(fetchStatsCmd(m), tickCmd(m.gen))
}

func fetchStatsCmd(m Model) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()
		s, err := m.client.Stats(ctx)
		return statsMsg{gen: m.gen, stats: s, err: err}
	}
}

// Update handles refresh keystrokes, stats deliveries, and watch ticks.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		cmds := []tea.Cmd{tickCmd(m.gen)}
		if m.watchEnabled && m.client != nil && !m.busy {
			m.busy = true
			cmds = append(cmds, fetchStatsCmd(m))
		}
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		if key.Matches(msg, m.watchKey) {
			m.watchEnabled = !m.watchEnabled
			return m, nil
		}
		if key.Matches(msg, m.refreshKey) && m.client != nil && !m.busy {
			m.busy = true
			return m, fetchStatsCmd(m)
		}
	case statsMsg:
		if msg.gen != m.gen {
			return m, nil
		}
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
	out := styles.Stat.Render(line)
	// Partial failure (RBAC denial on one resource, …): the counters render
	// anyway, with a dim note about which ones could not be fetched.
	if len(m.stats.Failed) > 0 {
		out += "\n" + styles.Hint.Render("unavailable: "+strings.Join(m.stats.Failed, ", "))
	}
	return out
}

// Title implements views.View. Stable routing name; watch state is in
// KubectlEquivalent's "--watch" suffix.
func (m Model) Title() string { return "dashboard" }

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if m.watchEnabled {
		return "kubectl get all -A --watch"
	}
	return "kubectl get all -A"
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.refreshKey, m.watchKey}
}

// CapturesKeys implements views.View. Dashboard has no input fields, so
// global navigation always wins.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. Dashboard owns no resources beyond the in-flight
// stats fetch (bounded by ctx + 5s timeout), so this is a no-op.
func (m Model) Close() error { return nil }
