// Package tuitest is the test harness for the Bubble Tea models under
// internal/tui: key constructors, a Cmd driver that stands in for
// tea.Program, ANSI stripping, golden files, goroutine baselines and a fake
// cluster.
//
// Tests outside this package never build framework key or message literals
// themselves. A framework upgrade (Bubble Tea v1 to v2) then edits this
// package instead of every test file.
package tuitest

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// altPrefix marks a key pressed together with Alt, as in "alt+x".
const altPrefix = "alt+"

// namedKeys maps every key name the framework prints (msg.String()) back to
// its key type, so Press accepts exactly the spellings key.Binding matches on.
var namedKeys = buildNamedKeys()

func buildNamedKeys() map[string]tea.KeyType {
	out := make(map[string]tea.KeyType, 128)
	// Special keys are negative, control characters are 0..127.
	for k := tea.KeyType(-256); k <= 127; k++ {
		name := k.String()
		if name == "" || k == tea.KeyRunes {
			continue
		}
		if _, dup := out[name]; !dup {
			out[name] = k
		}
	}
	out["space"] = tea.KeySpace
	return out
}

// Press returns the message for one key press. k is a key name as
// key.Binding spells it ("enter", "esc", "ctrl+c", "up", "pgdown",
// "space") or a single character ("e", "/", "?"). An "alt+" prefix sets the
// Alt modifier. An unknown multi-character name panics: it is a typo in the
// test, not a runtime condition.
func Press(k string) tea.Msg {
	return PressKey(k)
}

// PressKey is Press typed as the framework's key event, for components
// whose Update takes a key event rather than any message (filter.Model).
func PressKey(k string) tea.KeyMsg {
	if rest, ok := strings.CutPrefix(k, altPrefix); ok && rest != "" {
		msg := PressKey(rest)
		msg.Alt = true
		return msg
	}
	if kt, ok := namedKeys[k]; ok {
		if kt == tea.KeySpace {
			return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
		}
		return tea.KeyMsg{Type: kt}
	}
	if utf8.RuneCountInString(k) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
	}
	panic(fmt.Sprintf("tuitest: unknown key %q", k))
}

// Keys returns one Press message per name, in order.
func Keys(names ...string) []tea.Msg {
	out := make([]tea.Msg, 0, len(names))
	for _, n := range names {
		out = append(out, Press(n))
	}
	return out
}

// Type returns the message a terminal delivers when text arrives in one
// read: a single key event carrying every rune, the way Bubble Tea v1
// reports fast typing and pastes. Text inputs insert all of it; single-key
// bindings do not match it. Empty text panics.
func Type(text string) tea.Msg {
	return TypeKey(text)
}

// TypeKey is Type typed as the framework's key event.
func TypeKey(text string) tea.KeyMsg {
	if text == "" {
		panic("tuitest: Type needs text")
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

// Enter is Press("enter").
func Enter() tea.Msg { return Press("enter") }

// Esc is Press("esc").
func Esc() tea.Msg { return Press("esc") }

// Resize returns the message the terminal sends when it is width x height.
func Resize(width, height int) tea.Msg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// KeyName returns the name key bindings match msg against, and false when
// msg is not a key press.
func KeyName(msg tea.Msg) (string, bool) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return "", false
	}
	return k.String(), true
}

// HelpKeys lists the key labels of the bindings v advertises, in order.
func HelpKeys(v interface{ Help() []key.Binding }) []string {
	bindings := v.Help()
	out := make([]string, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, b.Help().Key)
	}
	return out
}

// FilterBy returns the keys that open a list view's "/" filter, type query
// and commit it, leaving only the matching rows with the cursor on the
// first one.
func FilterBy(query string) []tea.Msg {
	return []tea.Msg{Press("/"), Type(query), Enter()}
}
