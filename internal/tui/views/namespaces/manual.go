package namespaces

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// promptLimit is the longest namespace name, a DNS label.
const promptLimit = 63

var typeKey = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "type a namespace"))

// prompt is what the view offers when RBAC forbids listing namespaces: an
// input to type one, over the namespaces this session already used. The
// zero value is off.
type prompt struct {
	on    bool
	input textinput.Model
}

func newPrompt() prompt {
	ti := textinput.New()
	ti.Prompt = "namespace › "
	ti.Placeholder = "type a namespace, or pick one used this session"
	ti.CharLimit = promptLimit
	ti.SetWidth(promptLimit)
	ti.SetStyles(styles.TextInput())
	ti.Focus()
	return prompt{on: true, input: ti}
}

func (p prompt) focused() bool { return p.on && p.input.Focused() }

// known are the namespaces this session is known to use, each once: the
// kubeconfig context's, the one k4s started in, then those the root passed
// with WithRecent. Listing them needs no permission.
func (m Model) known() []k8s.Namespace {
	var names []string
	if m.client != nil {
		names = append(names, m.client.ContextNamespace, m.client.Namespace)
	}
	names = append(names, m.recent...)
	out := make([]k8s.Namespace, 0, len(names))
	seen := make([]string, 0, len(names))
	for _, n := range names {
		if n == "" || slices.Contains(seen, n) {
			continue
		}
		seen = append(seen, n)
		out = append(out, k8s.Namespace{Name: n})
	}
	return out
}

// enterPrompt turns a refused first list into the prompt: there are no
// rows to keep, so the known namespaces stand in for them, the first one
// under the cursor.
func (m Model) enterPrompt(err error) Model {
	if m.prompt.on || !m.health.Failed() || k8s.Classify(err).Kind != k8s.KindForbidden {
		return m
	}
	m.prompt = newPrompt()
	m.raw = m.known()
	m = m.withRows()
	if len(m.raw) > 0 {
		m = m.withCursor(1)
	}
	return m
}

// feedPrompt handles a key or a paste while the prompt is up. consumed is
// false for what the view's own bindings take: Enter on a row once the
// prompt has given the keyboard back, r, w.
func (m Model) feedPrompt(msg tea.Msg) (Model, tea.Cmd, bool) {
	if !m.prompt.on {
		return m, nil, false
	}
	k, isKey := msg.(tea.KeyPressMsg)
	if !m.prompt.focused() {
		if !isKey || !key.Matches(k, typeKey) {
			return m, nil, false
		}
		blink := m.prompt.input.Focus()
		return m, blink, true
	}
	switch {
	case isKey && keys.IsEnter(k):
		return m.submit()
	case isKey && keys.IsEsc(k):
		m.prompt.input.Blur()
		return m, nil, true
	case isKey && (k.String() == "up" || k.String() == "down"):
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd, true
	}
	var cmd tea.Cmd
	m.prompt.input, cmd = m.prompt.input.Update(msg)
	return m, cmd, true
}

// submit opens the typed namespace, or the row under the cursor when
// nothing is typed. The name is checked as a namespace name only: the
// user may not be allowed to look it up.
func (m Model) submit() (Model, tea.Cmd, bool) {
	name := m.prompt.input.Value()
	if name == "" {
		next, cmd, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		return next, cmd, true
	}
	if err := k8s.ValidateNamespace(name); err != nil {
		msg := views.StatusMsg{Level: status.Warn, Text: err.Error()}
		return m, func() tea.Msg { return msg }, true
	}
	return m, func() tea.Msg { return views.NamespaceSelectedMsg{Namespace: name} }, true
}

// promptView is the refusal, the prompt and the namespaces to pick from.
func (m Model) promptView() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		styles.Warn.Render("namespaces unavailable: "+stale.Describe(m.health.Err())),
		m.prompt.input.View(),
		m.table.View(),
	)
}

// promptHelp are the bindings while the prompt is up.
func (m Model) promptHelp() []key.Binding {
	watch := m.watchKey
	watch.SetHelp("w", m.poll.Hint())
	return []key.Binding{m.selectKey, typeKey, watch, m.refreshKey}
}
