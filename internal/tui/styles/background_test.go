package styles

import (
	"fmt"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestColorFGBGIsDark(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want bool
	}{
		{in: "", want: true},
		{in: "15", want: true},
		{in: "15;0", want: true},
		{in: "0;15", want: false},
		{in: "0;default;15", want: false},
		{in: "0;7", want: false},
		{in: "0;6", want: true},
		{in: "0;8", want: false},
		{in: "0;9", want: false},
		{in: "0;16", want: true},
		{in: "0;231", want: false},
		{in: "0;232", want: true},
		{in: "0;255", want: false},
		{in: "0;256", want: true},
		{in: "0;-1", want: true},
		{in: "0;abc", want: true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			t.Parallel()
			if got := colorFGBGIsDark(c.in); got != c.want {
				t.Errorf("colorFGBGIsDark(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestIsDarkColor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		c    color.Color
		want bool
	}{
		{name: "black", c: color.RGBA{A: 0xFF}, want: true},
		{name: "solarized dark", c: color.RGBA{R: 0x00, G: 0x2B, B: 0x36, A: 0xFF}, want: true},
		{name: "solarized light", c: color.RGBA{R: 0xFD, G: 0xF6, B: 0xE3, A: 0xFF}, want: false},
		{name: "white", c: color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, want: false},
		{name: "mid grey is light", c: color.RGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := isDarkColor(c.c); got != c.want {
				t.Errorf("isDarkColor = %v, want %v", got, c.want)
			}
		})
	}
}

// onTerminal makes stdout a terminal (or not) for one test. Tests that
// use it swap package state, so they run sequentially.
func onTerminal(t *testing.T, tty bool) {
	t.Helper()
	prev := stdoutIsTerminal
	t.Cleanup(func() {
		stdoutIsTerminal = prev
		resetTheme()
	})
	stdoutIsTerminal = func() bool { return tty }
}

// Before the terminal answers k4s guesses from COLORFGBG; the program asks
// with OSC 11 only where the answer belongs to this terminal.
func TestBackgroundPolicy(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		tty      bool
		wantDark bool
		wantAsk  bool
	}{
		{name: "a terminal is asked", env: map[string]string{"TERM": "xterm-256color"}, tty: true, wantDark: true, wantAsk: true},
		{name: "COLORFGBG guesses light until the answer", env: map[string]string{"TERM": "xterm", "COLORFGBG": "0;15"}, tty: true, wantDark: false, wantAsk: true},
		{name: "tmux is not asked", env: map[string]string{"TERM": "tmux-256color", "COLORFGBG": "0;15"}, tty: true, wantDark: false},
		{name: "screen is not asked", env: map[string]string{"TERM": "screen"}, tty: true, wantDark: true},
		{name: "dumb is not asked", env: map[string]string{"TERM": "dumb", "COLORFGBG": "0;15"}, tty: true, wantDark: false},
		{name: "not a terminal", env: map[string]string{"TERM": "xterm", "COLORFGBG": "0;15"}, tty: false, wantDark: true},
		{name: "CI", env: map[string]string{"TERM": "xterm", "CI": "true", "COLORFGBG": "0;15"}, tty: true, wantDark: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			onTerminal(t, c.tty)
			getenv := envOf(c.env)
			if got := guessDark(getenv); got != c.wantDark {
				t.Errorf("guessDark = %v, want %v", got, c.wantDark)
			}
			if got := canAskBackground(getenv); got != c.wantAsk {
				t.Errorf("canAskBackground = %v, want %v", got, c.wantAsk)
			}
		})
	}
}

// Only the themes that follow the terminal ask for its background.
func TestWantsBackground(t *testing.T) {
	cases := []struct {
		theme   string
		noColor bool
		want    bool
	}{
		{theme: "auto", want: true},
		{theme: "high-contrast", want: true},
		{theme: "dark"},
		{theme: "light"},
		{theme: "mono"},
		{theme: "auto", noColor: true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%s no_color=%v", c.theme, c.noColor), func(t *testing.T) {
			onTerminal(t, true)
			env := map[string]string{"TERM": "xterm-256color", "K4S_THEME": c.theme}
			if c.noColor {
				env["NO_COLOR"] = "1"
			}
			if err := Init(envOf(env)); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if got := WantsBackground(); got != c.want {
				t.Fatalf("WantsBackground = %v, want %v", got, c.want)
			}
		})
	}
}

func TestSetBackground(t *testing.T) {
	light := color.RGBA{R: 0xFD, G: 0xF6, B: 0xE3, A: 0xFF}
	type step struct {
		bg       color.Color
		changed  bool
		wantHint string
	}
	cases := []struct {
		name  string
		theme string
		steps []step
	}{
		{name: "auto follows the answer", theme: "auto", steps: []step{
			{bg: light, changed: true, wantHint: tokenMuted.Light},
			{bg: color.White, changed: false, wantHint: tokenMuted.Light},
			{bg: color.Black, changed: true, wantHint: tokenMuted.Dark},
		}},
		{name: "high contrast follows the answer", theme: "high-contrast", steps: []step{
			{bg: light, changed: true, wantHint: highContrastTokens.muted.Light},
		}},
		{name: "an explicit dark theme stays", theme: "dark", steps: []step{
			{bg: light, changed: false, wantHint: tokenMuted.Dark},
		}},
		{name: "no colour is no answer", theme: "auto", steps: []step{
			{bg: nil, changed: false, wantHint: tokenMuted.Dark},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			onTerminal(t, true)
			if err := Init(envOf(map[string]string{"TERM": "xterm-256color", "K4S_THEME": c.theme})); err != nil {
				t.Fatalf("Init: %v", err)
			}
			for i, s := range c.steps {
				if got := SetBackground(s.bg); got != s.changed {
					t.Errorf("step %d: SetBackground changed = %v, want %v", i, got, s.changed)
				}
				if got := Hint.GetForeground(); got != lipgloss.Color(s.wantHint) {
					t.Errorf("step %d: Hint fg = %v, want %s", i, got, s.wantHint)
				}
			}
		})
	}
}
