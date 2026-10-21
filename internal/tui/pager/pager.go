// Package pager is the scrollable text body that describe and YAML
// share: the text wrapped to the width, a scroll position for the title,
// and a "/" search that highlights its matches and steps through them
// with n and N.
//
// Lifecycle of a search:
//
//	'/'                → the prompt opens, prefilled with the last query
//	typing             → the prompt takes every key (CapturesKeys)
//	Enter              → the query is committed and the first match shown;
//	                     an empty query ends the search
//	Esc in the prompt  → the prompt closes, the committed query stays
//	n / N              → the next or previous match, wrapping around
//	ClearSearch        → the host's Esc: the search ends
//
// Matching ignores case. The text is drawn as given: callers pass text
// that sanitize.Text has made inert.
package pager

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	minHeight = 1
	// queryLimit bounds what the prompt takes; a longer needle is a paste
	// gone wrong, not a search.
	queryLimit = 128
)

var (
	scrollKey = key.NewBinding(
		key.WithKeys("up", "down", "j", "k", "pgup", "pgdown", "g", "G"),
		key.WithHelp("↑↓/pgup/pgdn", "scroll"),
	)
	searchKey = key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search"))
	nextKey   = key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev match"))
	prevKey   = key.NewBinding(key.WithKeys("N"))
	topKey    = key.NewBinding(key.WithKeys("g", "home"))
	bottomKey = key.NewBinding(key.WithKeys("G", "end"))
)

// Model is the pager. The zero value is not usable: call New.
type Model struct {
	vp     viewport.Model
	input  textinput.Model
	width  int
	height int

	// raw is the text as given; lines is raw wrapped to the width.
	raw   string
	lines []string

	open    bool
	query   string
	pattern *regexp.Regexp
	// matches are the indexes of the wrapped lines holding the query;
	// current indexes matches.
	matches []int
	current int
}

// New returns an empty pager.
func New() Model {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	// The text is wrapped to the width, so there is nothing to pan to.
	vp.SetHorizontalStep(0)
	in := textinput.New()
	in.Prompt = "/ "
	in.CharLimit = queryLimit
	in.SetStyles(styles.TextInput())
	return Model{vp: vp, input: in}
}

// SetSize fits the pager to width x height, the search bar included, and
// wraps the text again.
func (m Model) SetSize(width, height int) Model {
	m.width, m.height = width, max(height, minHeight)
	m.input.SetWidth(max(width-len(m.input.Prompt)-1, 1))
	m.vp.SetWidth(width)
	return m.rewrap()
}

// SetContent shows text from the top. A committed search carries over to
// the new text.
func (m Model) SetContent(text string) Model {
	m.raw = text
	m = m.rewrap()
	m.vp.GotoTop()
	return m
}

// CapturesKeys reports whether the search prompt is open; the host then
// hands it every key.
func (m Model) CapturesKeys() bool { return m.open }

// Searching reports a committed query: the host's Esc clears it first.
func (m Model) Searching() bool { return m.query != "" }

// ClearSearch ends the search and closes the prompt.
func (m Model) ClearSearch() Model {
	m.open, m.query, m.pattern = false, "", nil
	m.input.Blur()
	m.input.SetValue("")
	return m.rewrap()
}

// Help is the pager's keys for the footer and the help popup.
func (m Model) Help() []key.Binding {
	return []key.Binding{scrollKey, searchKey, nextKey}
}

// Update handles keys and pastes: the prompt's while it is open, the
// pager's own otherwise.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if !m.open {
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if m.open {
			return m.onPromptKey(msg)
		}
		return m.onKey(msg)
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m Model) onKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case key.Matches(msg, searchKey):
		m.open = true
		m.input.SetValue(m.query)
		m.input.CursorEnd()
		blink := m.input.Focus()
		return m.relayout(), blink
	case key.Matches(msg, nextKey):
		return m.step(1), nil
	case key.Matches(msg, prevKey):
		return m.step(-1), nil
	case key.Matches(msg, topKey):
		m.vp.GotoTop()
		return m, nil
	case key.Matches(msg, bottomKey):
		m.vp.GotoBottom()
		return m, nil
	}
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

func (m Model) onPromptKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch {
	case keys.IsEsc(msg):
		m.open = false
		m.input.Blur()
		return m.relayout(), nil
	case keys.IsEnter(msg):
		m.open = false
		m.input.Blur()
		return m.commit(strings.TrimSpace(m.input.Value())), nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// commit makes query the search and shows its first match.
func (m Model) commit(query string) Model {
	if query == "" {
		return m.ClearSearch()
	}
	m.query = query
	m.pattern = regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	m = m.rewrap()
	m.current = 0
	return m.show()
}

// step moves to the next (+1) or previous (-1) match, wrapping around.
func (m Model) step(dir int) Model {
	if len(m.matches) == 0 {
		return m
	}
	n := len(m.matches)
	m.current = ((m.current+dir)%n + n) % n
	return m.show()
}

// show scrolls the current match to a third of the way down.
func (m Model) show() Model {
	if len(m.matches) == 0 {
		return m
	}
	m.vp.SetYOffset(max(m.matches[m.current]-m.vp.Height()/3, 0))
	return m
}

// rewrap wraps the text to the width, finds the matches in the wrapped
// lines and hands the viewport the highlighted text.
func (m Model) rewrap() Model {
	content := m.raw
	if m.width > 0 {
		content = ansi.Wrap(content, m.width, "")
	}
	m.lines = strings.Split(content, "\n")
	m.matches = nil
	shown := m.lines
	if m.pattern != nil {
		shown = make([]string, len(m.lines))
		for i, l := range m.lines {
			shown[i] = l
			if m.pattern.MatchString(l) {
				m.matches = append(m.matches, i)
				shown[i] = m.pattern.ReplaceAllStringFunc(l, func(s string) string { return styles.Highlight.Render(s) })
			}
		}
	}
	m.current = min(m.current, max(len(m.matches)-1, 0))
	offset := m.vp.YOffset()
	m.vp.SetContentLines(shown)
	m.vp.SetYOffset(offset)
	return m.relayout()
}

// relayout gives the viewport the height the search bar leaves.
func (m Model) relayout() Model {
	h := m.height
	if m.bar() {
		h--
	}
	m.vp.SetHeight(max(h, minHeight))
	return m
}

func (m Model) bar() bool { return m.open || m.query != "" }

// View is the text, with the search bar under it while there is one.
func (m Model) View() string {
	if !m.bar() {
		return m.vp.View()
	}
	return m.vp.View() + "\n" + m.barView()
}

func (m Model) barView() string {
	if m.open {
		return m.input.View()
	}
	count := "no match"
	if n := len(m.matches); n > 0 {
		count = fmt.Sprintf("%d/%d", m.current+1, n)
	}
	text := "/ " + m.query + "  ·  " + count + "  ·  n/N next/prev  ·  esc clear"
	return styles.Hint.Render(ansi.Truncate(text, max(m.width, 1), "…"))
}

// Position is where the view is in the text, for the host's title: ""
// when it all fits, "top · ↓ more", "45%" or "end".
func (m Model) Position() string {
	switch {
	case m.vp.AtTop() && m.vp.AtBottom():
		return ""
	case m.vp.AtBottom():
		return "end"
	case m.vp.AtTop():
		return "top · ↓ more"
	}
	return fmt.Sprintf("%d%%", int(m.vp.ScrollPercent()*100))
}
