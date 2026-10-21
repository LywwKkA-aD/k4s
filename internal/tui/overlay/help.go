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

// HelpSection is a titled group of entries; its descriptions line up.
type HelpSection struct {
	Title   string
	Entries []HelpEntry
}

// help lists what the user can press right now.
type help struct {
	sections []HelpSection
	closeOn  []key.Binding
}

// NewHelp opens the help popup. Esc and the closeOn bindings close it; any
// other key or a paste closes it and is passed on (see Passes), so ':' with
// help open opens the command bar.
func NewHelp(sections []HelpSection, closeOn ...key.Binding) Overlay {
	return help{sections: sections, closeOn: closeOn}
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

// View is the title, each section with a blank line between, and the keys
// that close it.
func (h help) View(width int) string {
	lines := []string{styles.PopupTitle.Render("help"), ""}
	for i, sec := range h.sections {
		lines = append(lines, formatSection(sec))
		if i != len(h.sections)-1 {
			lines = append(lines, "")
		}
	}
	lines = append(lines, "", styles.Hint.Render("? / Esc close"))
	return rows(width, lines...)
}

// formatSection prints the title and the entries, padding the keys column
// so the descriptions align within the section.
func formatSection(sec HelpSection) string {
	width := 0
	for _, e := range sec.Entries {
		width = max(width, lipgloss.Width(e.Keys))
	}
	lines := make([]string, 0, len(sec.Entries)+1)
	lines = append(lines, styles.Title.Render(sec.Title))
	for _, e := range sec.Entries {
		pad := strings.Repeat(" ", width-lipgloss.Width(e.Keys))
		lines = append(lines, "  "+e.Keys+pad+"  "+styles.Hint.Render(e.Desc))
	}
	return strings.Join(lines, "\n")
}
