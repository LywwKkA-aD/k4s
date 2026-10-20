package keys_test

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/keys"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestPressPredicates(t *testing.T) {
	t.Parallel()

	kpEnter := tea.KeyPressMsg{Code: tea.KeyKpEnter}
	numLockUp := tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModNumLock}
	cases := []struct {
		name  string
		msg   tea.KeyPressMsg
		check func(tea.KeyPressMsg) bool
		want  bool
	}{
		{name: "enter", msg: tuitest.PressKey("enter"), check: keys.IsEnter, want: true},
		{name: "alt+enter is enter", msg: tuitest.PressKey("alt+enter"), check: keys.IsEnter, want: true},
		{name: "shift+enter is enter", msg: tuitest.PressKey("shift+enter"), check: keys.IsEnter, want: true},
		{name: "keypad enter is enter", msg: kpEnter, check: keys.IsEnter, want: true},
		{name: "e is not enter", msg: tuitest.PressKey("e"), check: keys.IsEnter, want: false},
		{name: "esc", msg: tuitest.PressKey("esc"), check: keys.IsEsc, want: true},
		{name: "alt+esc is esc", msg: tuitest.PressKey("alt+esc"), check: keys.IsEsc, want: true},
		{name: "q is not esc", msg: tuitest.PressKey("q"), check: keys.IsEsc, want: false},
		{name: "ctrl+c", msg: tuitest.PressKey("ctrl+c"), check: keys.IsForceQuit, want: true},
		{name: "ctrl+alt+c", msg: tuitest.PressKey("ctrl+alt+c"), check: keys.IsForceQuit, want: true},
		{name: "c is not force quit", msg: tuitest.PressKey("c"), check: keys.IsForceQuit, want: false},
		{name: "ctrl+d is not force quit", msg: tuitest.PressKey("ctrl+d"), check: keys.IsForceQuit, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.check(tc.msg); got != tc.want {
				t.Errorf("%s: got %v, want %v", tc.msg, got, tc.want)
			}
		})
	}

	arrows := []struct {
		name string
		msg  tea.KeyPressMsg
		code rune
		want bool
	}{
		{name: "up", msg: tuitest.PressKey("up"), code: tea.KeyUp, want: true},
		{name: "alt+up is up", msg: tuitest.PressKey("alt+up"), code: tea.KeyUp, want: true},
		{name: "num lock does not matter", msg: numLockUp, code: tea.KeyUp, want: true},
		{name: "shift+up is another key", msg: tuitest.PressKey("shift+up"), code: tea.KeyUp, want: false},
		{name: "ctrl+down is another key", msg: tuitest.PressKey("ctrl+down"), code: tea.KeyDown, want: false},
		{name: "down is not up", msg: tuitest.PressKey("down"), code: tea.KeyUp, want: false},
	}
	for _, tc := range arrows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := keys.IsArrow(tc.msg, tc.code); got != tc.want {
				t.Errorf("IsArrow(%s) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}
