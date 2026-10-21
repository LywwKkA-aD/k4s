package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// openHelp opens the help popup: every binding the user can act on right
// now. '?' and 'q' close it like Esc; any other key closes it and then acts.
func (m Model) openHelp() Model {
	m.overlay = overlay.NewHelp(m.helpSections(), m.keys.Help, m.keys.Quit)
	return m
}

// helpSections is the global keys, the view's own (if any), the view's
// map (if it has one and the popup still fits the body) and the commands,
// in that order.
func (m Model) helpSections() []overlay.HelpSection {
	sections := []overlay.HelpSection{{Title: "global", Entries: globalHelp(m)}}
	if vh := viewHelp(m.current.Help()); len(vh) > 0 {
		sections = append(sections, overlay.HelpSection{Title: "view: " + m.current.Title(), Entries: vh})
	}
	commands := overlay.HelpSection{Title: "commands", Entries: commandHelp()}
	if mp, ok := m.current.(mapper); ok {
		if entries := viewHelp(mp.HelpMap()); len(entries) > 0 {
			withMap := append(slices.Clip(sections), overlay.HelpSection{Title: m.current.Title() + " map", Entries: entries}, commands)
			if m.helpFits(withMap) {
				return withMap
			}
		}
	}
	return append(sections, commands)
}

// mapper is a view with a map of its parts for the help popup: the
// dashboard lists its panels with the key that focuses each and its
// kubectl command.
type mapper interface {
	HelpMap() []key.Binding
}

// helpFits reports whether the help popup with sections fits the body. The
// popup does not scroll, so a map that would push the commands off screen
// is left out.
func (m Model) helpFits(sections []overlay.HelpSection) bool {
	if m.height == 0 {
		return true
	}
	inner := m.width - styles.PopupBox.GetHorizontalFrameSize()
	box := styles.PopupBox.Render(overlay.NewHelp(sections).View(inner))
	return lipgloss.Height(box) <= m.bodyHeight()
}

// globalHelp lists the bindings the root model owns: quit/home, command bar,
// back, force-quit, YAML and copy, with the tmux setting copy needs. ? has
// no row: the popup's own last line says it closes the help, and the
// popup is cut to the body, so each global row costs a row of the view's
// section on a small terminal.
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
		{Desc: "in tmux: set -g set-clipboard on"},
	}
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
