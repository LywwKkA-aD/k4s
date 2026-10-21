package tui

import (
	"charm.land/bubbles/v2/key"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
)

// openHelp opens the help popup: every binding the user can act on right
// now. '?' and 'q' close it like Esc; any other key closes it and then acts.
func (m Model) openHelp() Model {
	m.overlay = overlay.NewHelp(m.helpSections(), m.keys.Help, m.keys.Quit)
	return m
}

// helpSections is the global keys, the view's own (if any) and the
// commands, in that order.
func (m Model) helpSections() []overlay.HelpSection {
	sections := []overlay.HelpSection{{Title: "global", Entries: globalHelp(m)}}
	if vh := viewHelp(m.current.Help()); len(vh) > 0 {
		sections = append(sections, overlay.HelpSection{Title: "view: " + m.current.Title(), Entries: vh})
	}
	return append(sections, overlay.HelpSection{Title: "commands", Entries: commandHelp()})
}

// globalHelp lists the bindings the root model owns: quit/home, command bar,
// back, force-quit, help itself and copy, with the tmux setting copy needs.
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
		{Keys: m.keys.Help.Help().Key, Desc: "this help"},
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
