// Help popup is a centered card that lists every binding the user can act
// on right now: global navigation, the active view's bindings, and the
// command-bar aliases. Rendered in place of the body when cmdMode is
// cmdBarHelp; the existing prompt-popup machinery handles the rest.
package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// helpEntry is one row inside a help section: a key cluster and its label.
type helpEntry struct {
	keys string
	desc string
}

// helpSection is a titled group of entries; the renderer aligns the keys
// column so the descriptions line up vertically inside the card.
type helpSection struct {
	title   string
	entries []helpEntry
}

// renderHelpPopupInner builds the help card body — title, global keys,
// view-specific keys (if any), and the command-bar aliases.
func (m Model) renderHelpPopupInner() string {
	sections := []helpSection{
		{title: "global", entries: globalHelp(m)},
	}
	if vh := viewHelp(m.current.Help()); len(vh) > 0 {
		sections = append(sections, helpSection{title: "view: " + m.current.Title(), entries: vh})
	}
	sections = append(sections, helpSection{title: "commands", entries: commandHelp()})

	rows := []string{styles.PopupTitle.Render("help"), ""}
	for i, sec := range sections {
		rows = append(rows, formatSection(sec))
		if i != len(sections)-1 {
			rows = append(rows, "")
		}
	}
	rows = append(rows, "", styles.Hint.Render("? / Esc close"))
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// globalHelp lists the bindings the root model owns: quit/home, command bar,
// back, force-quit, help itself.
func globalHelp(m Model) []helpEntry {
	quitDesc := "quit"
	if m.current.Title() != viewDashboard {
		quitDesc = "home (then quit on dashboard)"
	}
	return []helpEntry{
		{keys: m.keys.Quit.Help().Key, desc: quitDesc},
		{keys: m.keys.ForceQuit.Help().Key, desc: "force quit"},
		{keys: m.keys.Back.Help().Key, desc: "back"},
		{keys: m.keys.Command.Help().Key, desc: "command bar"},
		{keys: m.keys.Help.Help().Key, desc: "this help"},
	}
}

// viewHelp adapts the active view's bubbles/key bindings into the help-row
// shape; bindings without a help label are skipped silently.
func viewHelp(bindings []key.Binding) []helpEntry {
	out := make([]helpEntry, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		if h.Key == "" || h.Desc == "" {
			continue
		}
		out = append(out, helpEntry{keys: h.Key, desc: h.Desc})
	}
	return out
}

// commandHelp turns the command registry into a static list — the help
// popup shows what `:` accepts so users do not have to memorize the aliases.
func commandHelp() []helpEntry {
	names := command.All()
	out := make([]helpEntry, 0, len(names))
	for _, name := range names {
		out = append(out, helpEntry{keys: ":" + name, desc: command.Aliases(name)})
	}
	return out
}

// formatSection prints a section as title + key/desc rows, padding the keys
// column so the descriptions align inside this section.
func formatSection(sec helpSection) string {
	width := 0
	for _, e := range sec.entries {
		if w := lipgloss.Width(e.keys); w > width {
			width = w
		}
	}
	lines := make([]string, 0, len(sec.entries)+1)
	lines = append(lines, styles.Title.Render(sec.title))
	for _, e := range sec.entries {
		pad := strings.Repeat(" ", width-lipgloss.Width(e.keys))
		lines = append(lines, "  "+e.keys+pad+"  "+styles.Hint.Render(e.desc))
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}
