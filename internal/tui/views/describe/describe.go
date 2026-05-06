// Package describe is the scrollable describe view (kubectl describe-style).
package describe

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	fetchTimeout      = 5 * time.Second
	minViewportHeight = 5
	chromeReserve     = 7 // header + footer + bottom margin estimate
)

// Kind enumerates the resources the describe view supports.
type Kind string

// KindPod is the only kind in the MVP; deployment / service follow.
const KindPod Kind = "pod"

// Model is the describe view (scrollable text).
type Model struct {
	client    *k8s.Client
	kind      Kind
	namespace string
	name      string

	viewport     viewport.Model
	rawContent   string // last-fetched content, kept so we can re-wrap on resize
	contentWidth int    // width the viewport was last sized to
	err          error
	loaded       bool

	refreshKey key.Binding
}

type contentMsg struct {
	content string
	err     error
}

// New constructs a describe view for the given resource.
func New(client *k8s.Client, kind Kind, namespace, name string) Model {
	vp := viewport.New(80, 20)
	return Model{
		client:    client,
		kind:      kind,
		namespace: namespace,
		name:      name,
		viewport:  vp,
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// Init kicks off the first describe fetch.
func (m Model) Init() tea.Cmd {
	if m.client == nil {
		return nil
	}
	return fetchCmd(m.client, m.kind, m.namespace, m.name)
}

func fetchCmd(c *k8s.Client, kind Kind, ns, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		defer cancel()

		var (
			content string
			err     error
		)
		switch kind {
		case KindPod:
			content, err = c.DescribePod(ctx, ns, name)
		default:
			err = fmt.Errorf("unsupported kind: %s", kind)
		}
		return contentMsg{content: content, err: err}
	}
}

// Update handles size/refresh/content and forwards scroll keys to the viewport.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		m.viewport.Height = max(msg.Height-chromeReserve, minViewportHeight)
		m.contentWidth = msg.Width
		// Reflow the cached content for the new width — without this the
		// viewport keeps the original wrapping and lines clip on shrink.
		if m.rawContent != "" {
			m.viewport.SetContent(wrap(m.rawContent, m.contentWidth))
		}
	case tea.KeyMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil {
			return m, fetchCmd(m.client, m.kind, m.namespace, m.name)
		}
	case contentMsg:
		m.err = msg.err
		m.loaded = true
		if msg.err == nil {
			m.rawContent = msg.content
			m.viewport.SetContent(wrap(msg.content, m.contentWidth))
			m.viewport.GotoTop()
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// wrap reflows long lines to the given width using lipgloss's text wrapping.
// width <= 0 means "don't wrap" (we have not been sized yet).
func wrap(content string, width int) string {
	if width <= 0 {
		return content
	}
	return lipgloss.NewStyle().Width(width).Render(content)
}

// View renders the viewport or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading describe…")
	}
	if m.err != nil {
		return styles.Warn.Render("describe failed: " + m.err.Error())
	}
	return m.viewport.View()
}

// Title implements views.View.
func (m Model) Title() string {
	return string(m.kind) + " · " + m.name
}

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	return fmt.Sprintf("kubectl describe %s %s -n %s", m.kind, m.name, m.namespace)
}

// Help implements views.View.
func (m Model) Help() []key.Binding { return []key.Binding{m.refreshKey} }
