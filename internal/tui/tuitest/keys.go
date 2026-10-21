// Package tuitest is the test harness for the Bubble Tea models under
// internal/tui: key constructors, a Cmd driver that stands in for
// tea.Program, ANSI stripping, golden files, goroutine baselines and a fake
// cluster.
//
// Tests outside this package never build framework key or message literals
// themselves. A framework upgrade then edits this package instead of every
// test file.
package tuitest

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// modifiers are the prefixes Press accepts, as the framework prints them.
var modifiers = []struct {
	prefix string
	mod    tea.KeyMod
}{
	{prefix: "ctrl+", mod: tea.ModCtrl},
	{prefix: "alt+", mod: tea.ModAlt},
	{prefix: "shift+", mod: tea.ModShift},
}

// namedKeys maps every special key name the framework prints (msg.String())
// back to its key code, so Press accepts exactly the spellings key.Binding
// matches on.
var namedKeys = buildNamedKeys()

func buildNamedKeys() map[string]rune {
	codes := []rune{tea.KeyEnter, tea.KeyTab, tea.KeyBackspace, tea.KeyEscape, tea.KeySpace}
	// Special keys follow KeyExtended. The first code that prints a name
	// wins, so "up" is the arrow key and not its keypad twin.
	for c := tea.KeyExtended + 1; c <= tea.KeyExtended+512; c++ {
		codes = append(codes, c)
	}
	out := make(map[string]rune, len(codes))
	for _, c := range codes {
		name := tea.Key{Code: c}.String()
		if utf8.RuneCountInString(name) <= 1 {
			continue
		}
		if _, dup := out[name]; !dup {
			out[name] = c
		}
	}
	return out
}

// Press returns the message for one key press. k is a key name as
// key.Binding spells it ("enter", "esc", "ctrl+c", "up", "pgdown",
// "space") or a single character ("e", "/", "?"). A "ctrl+", "alt+" or
// "shift+" prefix sets that modifier. An unknown multi-character name
// panics: it is a typo in the test, not a runtime condition.
func Press(k string) tea.Msg {
	return PressKey(k)
}

// PressKey is Press typed as the framework's key event, for components
// whose Update takes a key event rather than any message.
func PressKey(k string) tea.KeyPressMsg {
	for _, m := range modifiers {
		if rest, ok := strings.CutPrefix(k, m.prefix); ok && rest != "" {
			msg := PressKey(rest)
			// A modified key carries no text, as the terminal reports it.
			msg.Text = ""
			msg.Mod |= m.mod
			return msg
		}
	}
	if code, ok := namedKeys[k]; ok {
		if code == tea.KeySpace {
			return tea.KeyPressMsg{Code: code, Text: " "}
		}
		return tea.KeyPressMsg{Code: code}
	}
	if utf8.RuneCountInString(k) == 1 {
		return printable([]rune(k)[0])
	}
	panic(fmt.Sprintf("tuitest: unknown key %q", k))
}

// printable is the key event a terminal sends for one printable character:
// an upper-case letter arrives as its lower-case key with Shift.
func printable(r rune) tea.KeyPressMsg {
	msg := tea.KeyPressMsg{Code: r, Text: string(r)}
	if unicode.IsUpper(r) {
		msg.Code = unicode.ToLower(r)
		msg.ShiftedCode = r
		msg.Mod |= tea.ModShift
	}
	return msg
}

// Keys returns one Press message per name, in order.
func Keys(names ...string) []tea.Msg {
	out := make([]tea.Msg, 0, len(names))
	for _, n := range names {
		out = append(out, Press(n))
	}
	return out
}

// Type returns one key event carrying every rune of text, the way the
// framework reports text that arrives as a single unit. Text inputs insert
// all of it; single-key bindings do not match it unless text is one rune.
// Empty text panics.
func Type(text string) tea.Msg {
	return TypeKey(text)
}

// TypeKey is Type typed as the framework's key event.
func TypeKey(text string) tea.KeyPressMsg {
	switch utf8.RuneCountInString(text) {
	case 0:
		panic("tuitest: Type needs text")
	case 1:
		return printable([]rune(text)[0])
	}
	return tea.KeyPressMsg{Code: tea.KeyExtended, Text: text}
}

// Paste returns the message a terminal sends when text is pasted with
// bracketed paste on.
func Paste(text string) tea.Msg {
	return tea.PasteMsg{Content: text}
}

// Colon returns the keys that run ":name" from the root's command bar.
func Colon(name string) []tea.Msg {
	return []tea.Msg{Press(":"), Type(name), Enter()}
}

// Enter is Press("enter").
func Enter() tea.Msg { return Press("enter") }

// Esc is Press("esc").
func Esc() tea.Msg { return Press("esc") }

// Resize returns the message the terminal sends when it is width x height.
func Resize(width, height int) tea.Msg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// Blur returns the message a terminal with focus reporting sends when its
// window loses focus.
func Blur() tea.Msg { return tea.BlurMsg{} }

// Focus returns the message a terminal with focus reporting sends when its
// window gains focus.
func Focus() tea.Msg { return tea.FocusMsg{} }

// KeyName returns the name key bindings match msg against, and false when
// msg is not a key press.
func KeyName(msg tea.Msg) (string, bool) {
	k, ok := msg.(tea.KeyPressMsg)
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
