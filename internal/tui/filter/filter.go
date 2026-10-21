// Package filter is the live "/ query" substring filter shared by every
// list view. It owns its textinput, exposes Match for callers to filter
// their raw items, and renders a top-of-body bar so the table layout below
// stays untouched.
//
// Lifecycle:
//
//	'/' (in the host view)      → Open()   focuses the input, prefilled with the current query
//	keystrokes inside the bar   → Update() query changes live
//	Enter                       → Commit() keeps the query and blurs the input
//	Enter on an empty value     → Close()  via Commit
//	Esc                         → Close()  clears the query
//
// While the input is focused '/' is an ordinary character. A committed query
// is cleared by reopening the bar with '/' and pressing Esc.
//
// Hosts call IsOpen to decide whether to forward keys to the filter or
// process them as their own bindings, and Active to decide whether to
// filter rows and draw the bar. Match reports whether the query is a
// case-insensitive substring of any of the supplied fields.
package filter

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// Model is the filter state. Zero value is not usable — call New().
type Model struct {
	input  textinput.Model
	open   bool
	query  string
	width  int
	prompt string
}

// New constructs a fresh filter with sensible defaults.
func New() Model {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 64
	ti.SetWidth(32)
	ti.SetStyles(styles.TextInput())
	return Model{
		input:  ti,
		prompt: "/ ",
	}
}

// SetWidth resizes the underlying input. Called from the host view when it
// receives a WindowSizeMsg so the bar stretches naturally.
func (m *Model) SetWidth(w int) {
	m.width = w
	inputW := w - len(m.prompt) - 2
	if inputW < 8 {
		inputW = 8
	}
	m.input.SetWidth(inputW)
}

// Open focuses the input. Returns a Cmd to start the cursor blink.
func (m Model) Open() (Model, tea.Cmd) {
	m.open = true
	m.input.SetValue(m.query)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

// Close clears the query and blurs the input. Use on Esc or when the user
// commits an empty value.
func (m Model) Close() Model {
	m.open = false
	m.query = ""
	m.input.SetValue("")
	m.input.Blur()
	return m
}

// Commit blurs the input but keeps the trimmed query, so rows stay filtered
// after the bar closes. An empty query closes the filter instead.
func (m Model) Commit() Model {
	m.query = strings.TrimSpace(m.input.Value())
	if m.query == "" {
		return m.Close()
	}
	m.open = false
	m.input.Blur()
	return m
}

// WithQuery returns m with q committed, as if it had been typed and
// confirmed with Enter: closed, filtering, its bar showing "/ q". A blank
// q closes the filter. Views use it to restore a views.State.
func (m Model) WithQuery(q string) Model {
	m.input.SetValue(q)
	return m.Commit()
}

// Update handles a key press or a paste while the input is focused.
// Returns the updated model, a Cmd from textinput, and a "consumed" flag the
// host uses to decide whether to also let its own bindings run.
//
// Hosts call it only while IsOpen reports true; on a closed filter, and for
// any other message, it changes nothing and reports the message as not
// consumed.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd, bool) {
	if !m.open {
		return m, nil, false
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if keys.IsEsc(msg) {
			return m.Close(), nil, true
		}
		if keys.IsEnter(msg) {
			return m.Commit(), nil, true
		}
	case tea.PasteMsg:
	default:
		return m, nil, false
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.query = strings.TrimSpace(m.input.Value())
	return m, cmd, true
}

// Active reports whether the filter is currently capturing keystrokes
// (open prompt) or whether a previous commit left a non-empty query in
// place. Either way the host should be filtering rows.
func (m Model) Active() bool { return m.open || m.query != "" }

// IsOpen reports whether the input is focused. Host views use it to route
// every keystroke to Update instead of their own bindings.
func (m Model) IsOpen() bool { return m.open }

// Query returns the current query as typed, trimmed of surrounding
// whitespace. Case is preserved; Match lowers both sides itself.
func (m Model) Query() string { return m.query }

// View renders the filter bar — empty when the filter is dormant so the
// host's normal body has full vertical space.
func (m Model) View() string {
	switch {
	case m.open:
		return m.input.View()
	case m.query != "":
		return "/ " + m.query
	}
	return ""
}

// Match reports whether the current query is a case-insensitive substring
// of any of the supplied fields. An empty query matches everything (so
// callers can call Match unconditionally).
func (m Model) Match(fields ...string) bool {
	if m.query == "" {
		return true
	}
	q := strings.ToLower(m.query)
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
