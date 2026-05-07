// Package command exposes the registry of TUI commands the user can invoke
// from the ":" command bar (':pods', ':ns', ':dashboard', ...).
package command

import "strings"

// Resolve returns the canonical command name for an alias the user typed.
// Matching is case-insensitive and tolerates surrounding whitespace. The
// second return is false when the input does not match any known command.
func Resolve(input string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(input))
	if s == "" {
		return "", false
	}
	canonical, ok := aliasIndex[s]
	return canonical, ok
}

// All returns every canonical command name. Used by tests and the help popup.
func All() []string {
	out := make([]string, 0, len(commands))
	for _, c := range commands {
		out = append(out, c.canonical)
	}
	return out
}

// Aliases returns the comma-separated list of alternate spellings for the
// canonical command name (excluding the canonical itself). Empty when the
// canonical has no extra aliases or is unknown.
func Aliases(canonical string) string {
	for _, c := range commands {
		if c.canonical != canonical {
			continue
		}
		extras := make([]string, 0, len(c.aliases))
		for _, a := range c.aliases {
			if a != canonical {
				extras = append(extras, a)
			}
		}
		return strings.Join(extras, ", ")
	}
	return ""
}

type entry struct {
	canonical string
	aliases   []string
}

// commands is the source of truth for the registry.
var commands = []entry{
	{canonical: "dashboard", aliases: []string{"dashboard", "home"}},
	{canonical: "pods", aliases: []string{"pods", "po", "pod"}},
	{canonical: "namespaces", aliases: []string{"namespaces", "ns"}},
	{canonical: "deployments", aliases: []string{"deployments", "deploy", "deployment", "dp"}},
	{canonical: "services", aliases: []string{"services", "svc", "service"}},
}

// aliasIndex is built from commands at package init time.
var aliasIndex = func() map[string]string {
	m := make(map[string]string, len(commands)*3)
	for _, c := range commands {
		for _, a := range c.aliases {
			m[a] = c.canonical
		}
	}
	return m
}()
