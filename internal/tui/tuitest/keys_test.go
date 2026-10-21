package tuitest_test

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestPressRoundTripsKeyNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{in: "enter", want: "enter"},
		{in: "esc", want: "esc"},
		{in: "ctrl+c", want: "ctrl+c"},
		{in: "ctrl+u", want: "ctrl+u"},
		{in: "up", want: "up"},
		{in: "down", want: "down"},
		{in: "left", want: "left"},
		{in: "right", want: "right"},
		{in: "pgup", want: "pgup"},
		{in: "pgdown", want: "pgdown"},
		{in: "home", want: "home"},
		{in: "end", want: "end"},
		{in: "tab", want: "tab"},
		{in: "backspace", want: "backspace"},
		{in: "delete", want: "delete"},
		{in: "space", want: "space"},
		{in: " ", want: "space"},
		{in: "shift+tab", want: "shift+tab"},
		{in: "ctrl+alt+c", want: "ctrl+alt+c"},
		{in: "e", want: "e"},
		{in: "N", want: "N"},
		{in: "/", want: "/"},
		{in: "?", want: "?"},
		{in: ":", want: ":"},
		{in: "1", want: "1"},
		{in: "alt+x", want: "alt+x"},
		{in: "alt+enter", want: "alt+enter"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got, ok := tuitest.KeyName(tuitest.Press(tc.in))
			if !ok {
				t.Fatalf("Press(%q) is not a key message", tc.in)
			}
			if got != tc.want {
				t.Errorf("Press(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPressMatchesGlobalBindings(t *testing.T) {
	t.Parallel()

	km := keys.Default()
	cases := []struct {
		name    string
		msg     any
		binding key.Binding
	}{
		{name: "quit", msg: tuitest.Press("q"), binding: km.Quit},
		{name: "force quit", msg: tuitest.Press("ctrl+c"), binding: km.ForceQuit},
		{name: "help", msg: tuitest.Press("?"), binding: km.Help},
		{name: "command", msg: tuitest.Press(":"), binding: km.Command},
		{name: "back", msg: tuitest.Esc(), binding: km.Back},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, _ := tuitest.KeyName(tc.msg)
			if !key.Matches(keyMsg(t, tc.msg), tc.binding) {
				t.Errorf("%q does not match the %s binding", name, tc.name)
			}
		})
	}
}

func TestTypeCarriesTheWholeText(t *testing.T) {
	t.Parallel()

	got, ok := tuitest.KeyName(tuitest.Type("pods"))
	if !ok || got != "pods" {
		t.Fatalf("Type(pods) = %q (key=%v), want one key event spelling pods", got, ok)
	}
	if !key.Matches(keyMsg(t, tuitest.Type("q")), keys.Default().Quit) {
		t.Error("Type of a single rune should behave like Press of that rune")
	}
}

func TestShortcuts(t *testing.T) {
	t.Parallel()

	if n, _ := tuitest.KeyName(tuitest.Enter()); n != "enter" {
		t.Errorf("Enter() = %q", n)
	}
	if n, _ := tuitest.KeyName(tuitest.Esc()); n != "esc" {
		t.Errorf("Esc() = %q", n)
	}
	seq := tuitest.Keys("down", "down", "enter")
	if len(seq) != 3 {
		t.Fatalf("Keys returned %d messages, want 3", len(seq))
	}
	if n, _ := tuitest.KeyName(seq[2]); n != "enter" {
		t.Errorf("Keys()[2] = %q, want enter", n)
	}
	if _, ok := tuitest.KeyName(tuitest.Resize(80, 24)); ok {
		t.Error("Resize must not be a key message")
	}
	for _, msg := range []tea.Msg{tuitest.Blur(), tuitest.Focus()} {
		if _, ok := tuitest.KeyName(msg); ok {
			t.Errorf("%T must not be a key message", msg)
		}
	}
	if _, ok := tuitest.Blur().(tea.BlurMsg); !ok {
		t.Errorf("Blur() = %T, want the framework's blur message", tuitest.Blur())
	}
	if _, ok := tuitest.Focus().(tea.FocusMsg); !ok {
		t.Errorf("Focus() = %T, want the framework's focus message", tuitest.Focus())
	}
}

func TestBadKeysPanic(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		fn   func()
	}{
		{name: "unknown name", fn: func() { tuitest.Press("entr") }},
		{name: "empty text", fn: func() { tuitest.Type("") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if recover() == nil {
					t.Error("expected a panic")
				}
			}()
			tc.fn()
		})
	}
}
