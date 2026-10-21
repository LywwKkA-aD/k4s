// Package yaml is the YAML view: `kubectl get <kind> <name> -n <ns> -o
// yaml`, built in-process from a client-go Get the way kubectl prints it,
// and read in the same pager as describe: scroll, / search, n and N.
package yaml

import (
	"context"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/pager"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	fetchTimeout      = 10 * time.Second
	minViewportHeight = 5
	// actFetch is the one action in flight: the Get.
	actFetch = "yaml"
)

// manifest is a fetched object's YAML.
type manifest struct{ text string }

// Model is the YAML view of one object.
type Model struct {
	client *k8s.Client
	ref    views.ResourceRef

	// tok names this instance and the fetch epoch; inFlight takes only the
	// answer to the fetch in flight.
	tok      async.Token
	inFlight async.InFlight

	pager  pager.Model
	err    error
	loaded bool

	refreshKey key.Binding
}

// New constructs the YAML view of ref. With a client its fetch is already
// in flight: Init sends it.
func New(client *k8s.Client, ref views.ResourceRef) Model {
	tok := async.New()
	var inFlight async.InFlight
	if client != nil {
		inFlight, _ = inFlight.Begin(actFetch, tok)
	}
	return Model{
		client:     client,
		ref:        ref,
		tok:        tok,
		inFlight:   inFlight,
		pager:      pager.New(),
		refreshKey: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
}

// Init sends the fetch.
func (m Model) Init() tea.Cmd {
	if !m.inFlight.Busy(actFetch) {
		return nil
	}
	return m.fetch()
}

func (m Model) fetch() tea.Cmd {
	c, r := m.client, m.ref
	return async.Run(m.tok, fetchTimeout, func(ctx context.Context) (manifest, error) {
		text, err := c.ObjectYAML(ctx, r.Kind, r.Namespace, r.Name)
		return manifest{text: text}, err
	})
}

// Update handles size, refresh and the fetched YAML; scroll and search
// keys go to the pager.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.pager = m.pager.SetSize(msg.Width, max(msg.Height, minViewportHeight))
		return m, nil
	case tea.KeyPressMsg:
		if !m.pager.CapturesKeys() && key.Matches(msg, m.refreshKey) && m.client != nil {
			return m.refresh()
		}
	case async.Result[manifest]:
		return m.onManifest(msg), nil
	}
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// refresh fetches again under the next epoch. A fetch already in flight
// wins: its answer is as fresh as a second one would be.
func (m Model) refresh() (views.View, tea.Cmd) {
	next := m.tok.Next()
	var ok bool
	if m.inFlight, ok = m.inFlight.Begin(actFetch, next); !ok {
		return m, nil
	}
	m.tok = next
	return m, m.fetch()
}

// onManifest takes the answer to the fetch in flight and drops any other.
// The YAML is made inert before it is laid out: an annotation can hold
// any bytes.
func (m Model) onManifest(msg async.Result[manifest]) Model {
	var ok bool
	if m.inFlight, ok = m.inFlight.End(actFetch, msg.Token); !ok {
		return m
	}
	m.err, m.loaded = msg.Err, true
	if msg.Err == nil {
		m.pager = m.pager.SetContent(sanitize.Text(msg.Value.text))
	}
	return m
}

// View renders the YAML or a placeholder.
func (m Model) View() string {
	switch {
	case m.client == nil:
		return styles.Warn.Render("no kubeconfig")
	case !m.loaded:
		return styles.Hint.Render("loading yaml…")
	case m.err != nil:
		return styles.Warn.Render("yaml failed: " + sanitize.Line(m.err.Error()))
	}
	return m.pager.View()
}

// Title implements views.View: the object and where the view is in it.
func (m Model) Title() string {
	base := m.ref.Kind + " · " + m.ref.Name
	if !m.loaded || m.err != nil {
		return base
	}
	base += " · yaml"
	if pos := m.pager.Position(); pos != "" {
		return base + " · " + pos
	}
	return base
}

// Command implements views.View: `kubectl get <kind> <name> -n <ns> -o
// yaml`, without -n for a cluster-scoped object. A name kubectl would
// misread gives no command at all.
func (m Model) Command() kubectl.Cmd {
	if kubectl.CheckSubdomain(m.ref.Kind, m.ref.Name) != nil {
		return kubectl.Cmd{}
	}
	args := []string{"get", m.ref.Kind, m.ref.Name}
	if m.ref.Namespace != "" {
		if kubectl.CheckLabel("namespace", m.ref.Namespace) != nil {
			return kubectl.Cmd{}
		}
		args = append(args, "-n", m.ref.Namespace)
	}
	return kubectl.New(append(args, "-o", "yaml")...)
}

// Help implements views.View.
func (m Model) Help() []key.Binding { return append(m.pager.Help(), m.refreshKey) }

// CapturesKeys implements views.View: the search prompt takes every key.
func (m Model) CapturesKeys() bool { return m.pager.CapturesKeys() }

// Back implements views.BackHandler: Esc clears a search before it leaves.
func (m Model) Back() (views.View, tea.Cmd, bool) {
	if !m.pager.Searching() {
		return m, nil, false
	}
	m.pager = m.pager.ClearSearch()
	return m, nil, true
}

// Selected implements views.Selection: the object shown, so o fetches it
// again.
func (m Model) Selected() (views.ResourceRef, bool) { return m.ref, true }

// Close implements views.View. Nothing outlives the view.
func (m Model) Close() error { return nil }
