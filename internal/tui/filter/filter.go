// Package filter is the live "/ query" substring filter shared by every
// list view. It owns its textinput, exposes Match for callers to filter
// their raw items, and renders a top-of-body bar so the table layout below
// stays untouched.
//
// Lifecycle:
//
//	'/' (in the host view)         → Open() — focuses the input
//	keystrokes inside the bar      → Update() — query changes live
//	Enter (with non-empty value)   → Commit() — keeps query, blurs input
//	Esc (or '/' on empty value)    → Close()  — clears query
//
// Hosts call Active to decide whether to forward keys to the filter or
// process them as their own bindings, and Query to know what to match
// against. Match is a small convenience that ANDs the query as a single
// case-insensitive substring across the supplied fields.
package filter

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
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
	ti.Width = 32
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
	m.input.Width = inputW
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

// Commit blurs the input but keeps the query so it remains active until the
// user explicitly clears it with Esc / '/'.
func (m Model) Commit() Model {
	m.query = strings.TrimSpace(m.input.Value())
	if m.query == "" {
		return m.Close()
	}
	m.open = false
	m.input.Blur()
	return m
}

// Update handles a key message while the input is focused. Returns the
// updated model, a Cmd from textinput, and a "consumed" flag the host uses
// to decide whether to also let its own bindings run.
//
// The host is expected to gate this behind Open() returning true.
func (m Model) Update(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.open {
		return m, nil, false
	}
	switch msg.Type {
	case tea.KeyEsc:
		return m.Close(), nil, true
	case tea.KeyEnter:
		return m.Commit(), nil, true
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

// Open reports whether the prompt is currently focused — host views use
// this to know whether '/' should re-open or commit.
func (m Model) IsOpen() bool { return m.open }

// Query returns the current substring filter (lower-case for callers'
// convenience — Match handles case-insensitivity already, but exposing it
// pre-lowered avoids surprises in any other downstream check).
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
