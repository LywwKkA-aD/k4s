// Package describe is the describe view: `kubectl describe <kind> <name>`
// run through the session's kubectl, exactly as the footer shows it, with
// the built-in describer as the fallback when kubectl is missing, fails or
// times out. The title says which one wrote the text.
package describe

import (
	"context"
	"fmt"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/pager"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	// fetchTimeout covers both calls of the built-in describer, the object
	// and its events, the second of which can be slow in a busy namespace.
	fetchTimeout      = 10 * time.Second
	minViewportHeight = 5
	// actDescribe is the one action in flight: the fetch.
	actDescribe = "describe"

	sourceBuiltIn = "built-in"
	sourceKubectl = "via kubectl"
)

// Kind enumerates the resources the describe view supports.
type Kind string

// Supported kinds.
const (
	KindPod        Kind = "pod"
	KindDeployment Kind = "deployment"
	KindService    Kind = "service"
)

// Model is the describe view.
type Model struct {
	client    *k8s.Client
	kubectl   Kubectl
	kind      Kind
	namespace string
	name      string

	// tok names this instance; every fetch runs under its next epoch, and
	// inFlight takes only the answer to the fetch in flight, so one meant
	// for another object never lands here.
	tok      async.Token
	inFlight async.InFlight

	pager  pager.Model
	err    error
	loaded bool
	// builtin is set when the text shown is the built-in describer's;
	// source names who wrote it, for the title.
	builtin bool
	source  string

	refreshKey key.Binding
}

// Option changes how New builds the view.
type Option func(Model) Model

// WithKubectl makes the view run `kubectl describe` through k before it
// falls back to the built-in describer.
func WithKubectl(k Kubectl) Option {
	return func(m Model) Model {
		m.kubectl = k
		return m
	}
}

// New constructs a describe view for the given resource. With a client its
// first fetch is already in flight: Init sends it.
func New(client *k8s.Client, kind Kind, namespace, name string, opts ...Option) Model {
	tok := async.New()
	var inFlight async.InFlight
	if client != nil {
		inFlight, _ = inFlight.Begin(actDescribe, tok)
	}
	m := Model{
		client:     client,
		kind:       kind,
		namespace:  namespace,
		name:       name,
		tok:        tok,
		inFlight:   inFlight,
		pager:      pager.New(),
		refreshKey: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	}
	for _, opt := range opts {
		m = opt(m)
	}
	return m
}

// Init sends the first describe fetch.
func (m Model) Init() tea.Cmd {
	if !m.inFlight.Busy(actDescribe) {
		return nil
	}
	return m.fetch()
}

// output is what a fetch wrote, and why kubectl did not write it.
type output struct {
	text    string
	version string
	builtin bool
	// kubectlErr is why kubectl did not describe; nil when it did or was
	// not asked to.
	kubectlErr error
}

// fetch describes the object under the current Token: kubectl first, the
// built-in describer when kubectl cannot.
func (m Model) fetch() tea.Cmd {
	k, c, kind, ns, name, cmd := m.kubectl, m.client, m.kind, m.namespace, m.name, m.command()
	return async.Run(m.tok, kubectlTimeout+fetchTimeout, func(ctx context.Context) (output, error) {
		var kubectlErr error
		if k.enabled() {
			text, version, err := k.describe(ctx, string(kind), name, cmd)
			if err == nil {
				return output{text: text, version: version}, nil
			}
			kubectlErr = err
		}
		text, err := builtIn(ctx, c, kind, ns, name)
		return output{text: text, builtin: true, kubectlErr: kubectlErr}, err
	})
}

func builtIn(ctx context.Context, c *k8s.Client, kind Kind, ns, name string) (string, error) {
	switch kind {
	case KindPod:
		return c.DescribePod(ctx, ns, name)
	case KindDeployment:
		return c.DescribeDeployment(ctx, ns, name)
	case KindService:
		return c.DescribeService(ctx, ns, name)
	}
	return "", fmt.Errorf("unsupported kind: %s", kind)
}

// Update handles size, refresh and content; scroll and search keys go to
// the pager. The incoming WindowSizeMsg is already the body's size.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.pager = m.pager.SetSize(msg.Width, max(msg.Height, minViewportHeight))
		return m, nil
	case tea.KeyPressMsg:
		if !m.pager.CapturesKeys() && key.Matches(msg, m.refreshKey) && m.client != nil {
			return m.refresh()
		}
	case async.Result[output]:
		return m.onContent(msg)
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
	if m.inFlight, ok = m.inFlight.Begin(actDescribe, next); !ok {
		return m, nil
	}
	m.tok = next
	return m, m.fetch()
}

// onContent takes the answer to the fetch in flight and drops any other:
// one from another describe view, or from a fetch this one has moved past.
// The text is made inert before it is laid out: kubectl prints what the
// cluster holds.
func (m Model) onContent(msg async.Result[output]) (Model, tea.Cmd) {
	var ok bool
	if m.inFlight, ok = m.inFlight.End(actDescribe, msg.Token); !ok {
		return m, nil
	}
	out := msg.Value
	m.err, m.loaded = msg.Err, true
	m.builtin = out.builtin
	m.source = sourceOf(out)
	if msg.Err == nil {
		// The view shows one object, so a later answer is the same text
		// refreshed and the reader stays where they were; the first lands
		// in an empty pager, at the top.
		m.pager = m.pager.Refresh(sanitize.Text(out.text))
	}
	return m, fallbackStatus(out.kubectlErr)
}

func sourceOf(out output) string {
	switch {
	case out.builtin:
		return sourceBuiltIn
	case out.version == "":
		return sourceKubectl
	}
	return sourceKubectl + " " + out.version
}

// fallbackStatus says why kubectl did not describe; nothing when it did.
func fallbackStatus(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	text, missing := fallbackNote(err)
	level := status.Warn
	if missing {
		level = status.Info
	}
	msg := views.StatusMsg{Level: level, Text: text}
	return func() tea.Msg { return msg }
}

// View renders the text or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if !m.loaded {
		return styles.Hint.Render("loading describe…")
	}
	if m.err != nil {
		return styles.Warn.Render("describe failed: " + sanitize.Line(m.err.Error()))
	}
	return m.pager.View()
}

// Title implements views.View: the object, who described it and where
// the view is in the text.
func (m Model) Title() string {
	base := string(m.kind) + " · " + m.name
	if !m.loaded || m.err != nil {
		return base
	}
	base += " · " + m.source
	if pos := m.pager.Position(); pos != "" {
		return base + " · " + pos
	}
	return base
}

// Command implements views.View: the command the view runs. It is
// approximate once the built-in describer has written the text, which
// leaves out what kubectl would show.
func (m Model) Command() kubectl.Cmd {
	c := m.command()
	c.Approx = m.loaded && m.err == nil && m.builtin
	return c
}

func (m Model) command() kubectl.Cmd {
	return kubectl.New("describe", string(m.kind), m.name, "-n", m.namespace)
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

// Close implements views.View. No long-lived resources held.
func (m Model) Close() error { return nil }
