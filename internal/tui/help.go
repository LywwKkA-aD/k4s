package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// openHelp opens the help popup: every binding the user can act on right
// now. '?' and 'q' close it like Esc; any other key closes it and then acts.
func (m Model) openHelp() Model {
	m.overlay = overlay.NewHelp(m.helpPage(), m.keys.Help, m.keys.Quit)
	return m
}

// helpPage is the first of helpPages that fits the body, else the
// tightest, which renderPopup cuts inside its border.
func (m Model) helpPage() overlay.HelpPage {
	pages := m.helpPages()
	for _, p := range pages[:len(pages)-1] {
		if m.helpFits(p) {
			return p
		}
	}
	return pages[len(pages)-1]
}

// helpPages are the help popup from fullest to tightest: the global keys,
// the view's own (if any), the view's map (if it has one) and the
// commands. The popup does not scroll, so on a short body it gives up, in
// this order:
//
//  1. the commands' aliases: one row names the views ":" opens;
//  2. the view's map;
//  3. the tmux note under y;
//  4. the global keys' rows: one row names them, as the footer does;
//  5. the blank rows between sections.
//
// The view's own keys stay whole to the last, since the footer's hints
// lose them first on a small terminal.
func (m Model) helpPages() []overlay.HelpPage {
	var view, viewMap []overlay.HelpSection
	if vh := viewHelp(m.current.Help()); len(vh) > 0 {
		view = []overlay.HelpSection{{Title: "view: " + m.current.Title(), Entries: vh}}
	}
	if mp, ok := m.current.(mapper); ok {
		if entries := viewHelp(mp.HelpMap()); len(entries) > 0 {
			viewMap = []overlay.HelpSection{{Title: m.current.Title() + " map", Entries: entries}}
		}
	}
	global := overlay.HelpSection{Title: "global", Entries: globalHelp(m)}
	noted := overlay.HelpSection{Title: "global", Entries: append(globalHelp(m), tmuxNote)}
	commands, oneRow := overlay.HelpSection{Title: "commands", Entries: commandHelp()}, commandRow()
	page := func(tight bool, sections ...[]overlay.HelpSection) overlay.HelpPage {
		return overlay.HelpPage{Sections: slices.Concat(sections...), Tight: tight}
	}
	one := func(s overlay.HelpSection) []overlay.HelpSection { return []overlay.HelpSection{s} }
	return []overlay.HelpPage{
		page(false, one(noted), view, viewMap, one(commands)),
		page(false, one(noted), view, viewMap, one(oneRow)),
		page(false, one(noted), view, one(oneRow)),
		page(false, one(global), view, one(oneRow)),
		page(false, one(globalRow(m)), view, one(oneRow)),
		page(true, one(globalRow(m)), view, one(oneRow)),
	}
}

// mapper is a view with a map of its parts for the help popup: the
// dashboard lists its panels with the key that focuses each and its
// kubectl command.
type mapper interface {
	HelpMap() []key.Binding
}

// helpFits reports whether the help popup on page fits the body.
func (m Model) helpFits(page overlay.HelpPage) bool {
	if m.height == 0 {
		return true
	}
	inner := m.width - styles.PopupBox.GetHorizontalFrameSize()
	box := styles.PopupBox.Render(overlay.NewHelp(page).View(inner))
	return lipgloss.Height(box) <= m.bodyHeight()
}

// tmuxNote is the setting y needs in tmux, under the global keys.
var tmuxNote = overlay.HelpEntry{Desc: "in tmux: set -g set-clipboard on"}

// globalHelp lists the bindings the root model owns: quit/home, command bar,
// back, force-quit, YAML and copy. ? has no row: the popup's title row says
// it closes the help, and each global row costs a row of the view's section
// on a small terminal.
func globalHelp(m Model) []overlay.HelpEntry {
	quitDesc := "quit"
	if m.current.Title() != viewDashboard {
		quitDesc = "home (then quit on dashboard)"
	}
	return []overlay.HelpEntry{
		{Keys: m.keys.Quit.Help().Key, Desc: quitDesc},
		{Keys: m.keys.ForceQuit.Help().Key, Desc: "force quit"},
		{Keys: m.keys.Back.Help().Key, Desc: "back"},
		{Keys: m.keys.Command.Help().Key, Desc: "command bar"},
		{Keys: m.keys.YAML.Help().Key, Desc: "YAML of the selected object"},
		{Keys: m.keys.Copy.Help().Key, Desc: "copy the kubectl command below (OSC 52)"},
	}
}

// globalRow is the global keys on one row, in the footer's words.
func globalRow(m Model) overlay.HelpSection {
	quit := "quit"
	if m.current.Title() != viewDashboard {
		quit = "home"
	}
	return overlay.HelpSection{Title: "global", Inline: true, Entries: []overlay.HelpEntry{
		{Keys: m.keys.Quit.Help().Key, Desc: quit},
		{Keys: "^c", Desc: "quit"},
		{Keys: m.keys.Back.Help().Key, Desc: "back"},
		{Keys: m.keys.Command.Help().Key, Desc: "command"},
		{Keys: m.keys.YAML.Help().Key, Desc: "yaml"},
		{Keys: m.keys.Copy.Help().Key, Desc: "copy"},
	}}
}

// viewHelp adapts the active view's bubbles/key bindings into help rows;
// bindings without a help label are skipped silently.
func viewHelp(bindings []key.Binding) []overlay.HelpEntry {
	out := make([]overlay.HelpEntry, 0, len(bindings))
	for _, b := range bindings {
		h := b.Help()
		if h.Key == "" || h.Desc == "" {
			continue
		}
		out = append(out, overlay.HelpEntry{Keys: h.Key, Desc: h.Desc})
	}
	return out
}

// commandWords are the spellings the one-row commands section names a
// view by, as the dashboard's enter hints do (":deploy"); a command not
// here goes by its own name.
var commandWords = map[string]string{
	"dashboard":   "home",
	"namespaces":  "ns",
	"deployments": "deploy",
	"services":    "svc",
	"contexts":    "ctx",
	"forwards":    "pf",
}

// commandRow is the commands section on one row: ":" and what it opens.
func commandRow() overlay.HelpSection {
	names := command.All()
	words := make([]string, 0, len(names))
	for _, name := range names {
		if w, ok := commandWords[name]; ok {
			name = w
		}
		words = append(words, name)
	}
	return overlay.HelpSection{Inline: true, Entries: []overlay.HelpEntry{{Keys: ":", Desc: strings.Join(words, " ")}}}
}

// commandHelp turns the command registry into a static list — the help
// popup shows what `:` accepts so users do not have to memorize the aliases
// — followed by the commands that take a namespace.
func commandHelp() []overlay.HelpEntry {
	names := command.All()
	out := make([]overlay.HelpEntry, 0, len(names)+2)
	for _, name := range names {
		out = append(out, overlay.HelpEntry{Keys: ":" + name, Desc: command.Aliases(name)})
	}
	return append(out,
		overlay.HelpEntry{Keys: ":ns <name>", Desc: "switch namespace (:ns -A for all)"},
		overlay.HelpEntry{Keys: ":<list> <ns>", Desc: "pods, deploy, svc, top, events; -A all"},
	)
}
