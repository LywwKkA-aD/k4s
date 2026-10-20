// Package styles owns every lipgloss style used by the TUI so that visual
// changes are localised and themeable in one place. Init picks the theme
// from the environment; until then the styles use the dark variants.
package styles

import (
	"charm.land/bubbles/v2/table"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// Styles shared by every view. Init rebuilds them for the chosen theme, so
// views read them when they render or construct a component, never from a
// copy taken at package init.
var (
	// Title is the bold accent heading, e.g. "k4s" in the header.
	Title lipgloss.Style
	// Subtitle is secondary text under a heading.
	Subtitle lipgloss.Style
	// Hint is helper text: key hints, loading and empty-state notes.
	Hint lipgloss.Style
	// OK marks success and the current choice in pickers.
	OK lipgloss.Style
	// Warn marks warnings.
	Warn lipgloss.Style
	// Err marks errors.
	Err lipgloss.Style
	// Stat is a bordered stat card on the dashboard.
	Stat lipgloss.Style
	// Header is the top bar, ruled off from the body.
	Header lipgloss.Style

	// Footer reserves an extra line at the bottom (PaddingBottom) so the
	// chrome does not sit directly on the terminal edge — handy for tmux
	// status bars and so the kubectl hint never hugs the system prompt.
	Footer lipgloss.Style

	// KubectlHint is the "≈ kubectl ..." line in the footer.
	KubectlHint lipgloss.Style

	// Highlight is used for search matches in the logs view.
	Highlight lipgloss.Style

	// PopupBox is the bordered card used by interactive prompts (tail
	// lines, container picker) so they pop over the body instead of
	// hiding among the footer hints.
	PopupBox lipgloss.Style

	// PopupTitle is the bold accent header inside a popup.
	PopupTitle lipgloss.Style

	// Placeholder is for text input placeholders. The bubbles default is
	// ANSI 240, about 2:1 on Solarized Dark.
	Placeholder lipgloss.Style
)

// active is the palette the styles were last built from.
var active palette

func init() { use(colourPalette(true)) }

// use rebuilds every style from p. The mono attributes (bold, underline,
// reverse) only switch on when p has no colours to carry state.
func use(p palette) {
	active = p
	Title = lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	Subtitle = lipgloss.NewStyle().Foreground(p.muted).Italic(true)
	Hint = lipgloss.NewStyle().Foreground(p.muted)
	OK = lipgloss.NewStyle().Foreground(p.ok).Bold(p.mono)
	Warn = lipgloss.NewStyle().Foreground(p.warn).Bold(p.mono)
	Err = lipgloss.NewStyle().Foreground(p.err).Bold(true).Underline(p.mono)
	Stat = lipgloss.NewStyle().
		Foreground(p.accent).
		Bold(true).
		Padding(0, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.muted)
	Header = lipgloss.NewStyle().
		Padding(0, 1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(p.muted)
	Footer = lipgloss.NewStyle().
		Padding(0, 1, 1, 1).
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(p.muted)
	KubectlHint = lipgloss.NewStyle().Foreground(p.accent).Italic(true)
	Highlight = lipgloss.NewStyle().
		Foreground(p.matchFg).
		Background(p.matchBg).
		Bold(true).
		Reverse(p.mono)
	PopupBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(p.accent).
		Padding(1, 3)
	PopupTitle = lipgloss.NewStyle().Bold(true).Foreground(p.accent)
	Placeholder = lipgloss.NewStyle().Foreground(p.muted).Italic(p.mono)
}

// Table returns the table styles used across views.
func Table() table.Styles {
	s := table.DefaultStyles()
	s.Header = s.Header.
		Foreground(active.accent).
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(active.muted).
		BorderBottom(true)
	// Built from scratch: the bubbles default carries an ANSI 212
	// foreground that would leak into mono.
	s.Selected = lipgloss.NewStyle().
		Foreground(active.selectionFg).
		Background(active.selectionBg).
		Bold(true).
		Reverse(active.mono)
	return s
}

// FitTable sizes t to the width its columns render at. A bubbles v2 table
// renders no rows at width 0, which is what an unsized bubbles v1 table
// treated as "as wide as the columns"; rows wider than the screen still
// wrap in the body as before.
func FitTable(t *table.Model) {
	pad := Table().Cell.GetHorizontalFrameSize()
	w := 0
	for _, c := range t.Columns() {
		if c.Width > 0 {
			w += c.Width + pad
		}
	}
	t.SetWidth(w)
}

// TextInput returns the text input styles used by the command bar, the
// prompts, the list filter and the logs search. Prompt and text stay
// unstyled and the cursor is a plain reverse-video block: the bubbles
// defaults paint the prompt, the cursor and blurred text in ANSI colours
// that would leak into mono.
func TextInput() textinput.Styles {
	state := textinput.StyleState{
		Text:        lipgloss.NewStyle(),
		Placeholder: Placeholder,
		Suggestion:  Placeholder,
		Prompt:      lipgloss.NewStyle(),
	}
	return textinput.Styles{
		Focused: state,
		Blurred: state,
		Cursor: textinput.CursorStyle{
			Shape: tea.CursorBlock,
			Blink: true,
		},
	}
}
