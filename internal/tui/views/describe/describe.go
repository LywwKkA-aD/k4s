// Package describe is the scrollable describe view (kubectl describe-style).
package describe

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// fetchTimeout covers both calls of a describe, the object and its
	// events, the second of which can be slow in a busy namespace.
	fetchTimeout      = 10 * time.Second
	minViewportHeight = 5
	// actDescribe is the one action in flight: the fetch.
	actDescribe = "describe"
)

// Kind enumerates the resources the describe view supports.
type Kind string

// Supported kinds.
const (
	KindPod        Kind = "pod"
	KindDeployment Kind = "deployment"
	KindService    Kind = "service"
)

// Model is the describe view (scrollable text).
type Model struct {
	client    *k8s.Client
	kind      Kind
	namespace string
	name      string

	// tok names this instance; every fetch runs under its next epoch, and
	// inFlight takes only the answer to the fetch in flight, so one meant
	// for another object never lands here.
	tok      async.Token
	inFlight async.InFlight

	viewport     viewport.Model
	rawContent   string // last-fetched content, kept so we can re-wrap on resize
	contentWidth int    // width the viewport was last sized to
	err          error
	loaded       bool

	scrollKey  key.Binding
	refreshKey key.Binding
}

// New constructs a describe view for the given resource. With a client its
// first fetch is already in flight: Init sends it.
func New(client *k8s.Client, kind Kind, namespace, name string) Model {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	// Content is wrapped to the width, so there is nothing to pan to, and
	// h/l must not pan the text off its first columns.
	vp.SetHorizontalStep(0)
	tok := async.New()
	var inFlight async.InFlight
	if client != nil {
		inFlight, _ = inFlight.Begin(actDescribe, tok)
	}
	return Model{
		client:    client,
		kind:      kind,
		namespace: namespace,
		name:      name,
		tok:       tok,
		inFlight:  inFlight,
		viewport:  vp,
		// Display-only binding so the footer surfaces scroll affordance —
		// the actual j/k/pgup/pgdn handling lives inside viewport.Update.
		scrollKey: key.NewBinding(
			key.WithKeys("up", "down", "j", "k", "pgup", "pgdown"),
			key.WithHelp("↑↓/pgup/pgdn", "scroll"),
		),
		refreshKey: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
	}
}

// Init sends the first describe fetch.
func (m Model) Init() tea.Cmd {
	if !m.inFlight.Busy(actDescribe) {
		return nil
	}
	return m.fetch()
}

// fetch describes the object under the current Token.
func (m Model) fetch() tea.Cmd {
	c, kind, ns, name := m.client, m.kind, m.namespace, m.name
	return async.Run(m.tok, fetchTimeout, func(ctx context.Context) (string, error) {
		switch kind {
		case KindPod:
			return c.DescribePod(ctx, ns, name)
		case KindDeployment:
			return c.DescribeDeployment(ctx, ns, name)
		case KindService:
			return c.DescribeService(ctx, ns, name)
		}
		return "", fmt.Errorf("unsupported kind: %s", kind)
	})
}

// Update handles size/refresh/content and forwards scroll keys to the viewport.
//
// The incoming WindowSizeMsg.Height has already been adjusted by the root
// model to be the body area only — no chrome math here.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		m.viewport.SetHeight(max(msg.Height, minViewportHeight))
		m.contentWidth = msg.Width
		// Reflow the cached content for the new width — without this the
		// viewport keeps the original wrapping and lines clip on shrink.
		if m.rawContent != "" {
			m.viewport.SetContent(wrap(m.rawContent, m.contentWidth))
		}
	case tea.KeyPressMsg:
		if key.Matches(msg, m.refreshKey) && m.client != nil {
			return m.refresh()
		}
	case async.Result[string]:
		return m.onContent(msg), nil
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// refresh fetches again under the next epoch. A fetch already in flight
// wins: its answer is as fresh as a second one would be.
func (m Model) refresh() (views.View, tea.Cmd) {
	next := m.tok.Next()
	var ok bool
	if m.inFlight, ok = m.inFlight.Begin(actDescribe, next); !ok {
		return m, nil
	}
	m.tok = next
	return m, m.fetch()
}

// onContent takes the answer to the fetch in flight and drops any other:
// one from another describe view, or from a fetch this one has moved past.
func (m Model) onContent(msg async.Result[string]) Model {
	var ok bool
	if m.inFlight, ok = m.inFlight.End(actDescribe, msg.Token); !ok {
		return m
	}
	m.err = msg.Err
	m.loaded = true
	if msg.Err == nil {
		m.rawContent = msg.Value
		m.viewport.SetContent(wrap(msg.Value, m.contentWidth))
		m.viewport.GotoTop()
	}
	return m
}

// wrap reflows long lines to the given width. lipgloss.Width().Render() does
// NOT word-wrap reliably (it pads short lines and clips long ones) — use the
// ANSI-aware wrapper from charmbracelet/x/ansi instead. width <= 0 means
// "don't wrap" (we have not been sized yet).
func wrap(content string, width int) string {
	if width <= 0 {
		return content
	}
	return ansi.Wrap(content, width, "")
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

// Title implements views.View. Appends a scroll indicator so the user knows
// (a) there is more content below and (b) where they are in it.
func (m Model) Title() string {
	base := string(m.kind) + " · " + m.name
	if !m.loaded || m.err != nil {
		return base
	}
	pct := int(m.viewport.ScrollPercent() * 100)
	switch {
	case m.viewport.AtTop() && m.viewport.AtBottom():
		// Content fits — nothing to scroll.
		return base
	case m.viewport.AtBottom():
		return base + " · end"
	case m.viewport.AtTop():
		return base + " · top · ↓ more"
	default:
		return fmt.Sprintf("%s · %d%%", base, pct)
	}
}

// Command implements views.View.
func (m Model) Command() kubectl.Cmd {
	return kubectl.New("describe", string(m.kind), m.name, "-n", m.namespace)
}

// Help implements views.View.
func (m Model) Help() []key.Binding { return []key.Binding{m.scrollKey, m.refreshKey} }

// CapturesKeys implements views.View. Describe is read-only — global keys
// pass through.
func (m Model) CapturesKeys() bool { return false }

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
