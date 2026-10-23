package overlay

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// HelpEntry is one help row: a key cluster and what it does.
type HelpEntry struct {
	Keys string
	Desc string
}

// HelpSection is a titled group of entries; its descriptions line up. A
// section without a title has no title row. An Inline one is a single
// row: its title, then each entry's keys and description.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
	Inline  bool
}

// HelpPage is what the help popup lists: its sections, with a blank row
// between them unless Tight.
type HelpPage struct {
	Sections []HelpSection
	Tight    bool
}

// help lists what the user can press right now.
type help struct {
	page    HelpPage
	closeOn []key.Binding
}

// NewHelp opens the help popup on page. Esc and the closeOn bindings close
// it; any other key or a paste closes it and is passed on (see Passes), so
// ':' with help open opens the command bar.
func NewHelp(page HelpPage, closeOn ...key.Binding) Overlay {
	return help{page: page, closeOn: closeOn}
}

// Update closes on any key or paste.
func (h help) Update(msg tea.Msg) (Overlay, tea.Cmd) {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.PasteMsg:
		return nil, nil
	}
	return h, nil
}

// PassesOn reports the input help closes on without using it.
func (h help) PassesOn(msg tea.Msg) bool {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return !h.closes(msg)
	case tea.PasteMsg:
		return true
	}
	return false
}

func (h help) closes(k tea.KeyPressMsg) bool {
	if keys.IsEsc(k) {
		return true
	}
	for _, b := range h.closeOn {
		if key.Matches(k, b) {
			return true
		}
	}
	return false
}

// View is the title with the keys that close it, then the sections. The
// close hint is on the first row, so a popup cut at the bottom to fit a
// short terminal still says how to close it.
func (h help) View(width int) string {
	lines := []string{styles.PopupTitle.Render("help") + "   " + styles.Hint.Render("? / Esc close")}
	for _, sec := range h.page.Sections {
		if !h.page.Tight {
			lines = append(lines, "")
		}
		lines = append(lines, formatSection(sec))
	}
	return rows(width, lines...)
}

// formatSection prints the title and the entries, padding the keys column
// so the descriptions align within the section; an inline section on one
// row.
func formatSection(sec HelpSection) string {
	if sec.Inline {
		return inlineSection(sec)
	}
	width := 0
	for _, e := range sec.Entries {
		width = max(width, lipgloss.Width(e.Keys))
	}
	lines := make([]string, 0, len(sec.Entries)+1)
	if sec.Title != "" {
		lines = append(lines, styles.Title.Render(sec.Title))
	}
	for _, e := range sec.Entries {
		pad := strings.Repeat(" ", width-lipgloss.Width(e.Keys))
		lines = append(lines, "  "+e.Keys+pad+"  "+styles.Hint.Render(e.Desc))
	}
	return strings.Join(lines, "\n")
}

// inlineSection is sec on one row: "global  q quit  ^c force quit".
func inlineSection(sec HelpSection) string {
	parts := make([]string, 0, len(sec.Entries)+1)
	if sec.Title != "" {
		parts = append(parts, styles.Title.Render(sec.Title))
	}
	for _, e := range sec.Entries {
		parts = append(parts, e.Keys+" "+styles.Hint.Render(e.Desc))
	}
	return strings.Join(parts, "  ")
}
