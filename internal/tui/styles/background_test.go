package styles

import (
	"errors"
	"image/color"
	"testing"
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

// The detectDark tests swap package hooks, so they run sequentially.
func TestDetectDark(t *testing.T) {
	light := color.RGBA{R: 0xFD, G: 0xF6, B: 0xE3, A: 0xFF}
	cases := []struct {
		name      string
		env       map[string]string
		tty       bool
		windows   bool
		reply     color.Color
		replyErr  error
		want      bool
		wantQuery bool
	}{
		{name: "light reply", env: map[string]string{"TERM": "xterm-256color"}, tty: true, reply: light, want: false, wantQuery: true},
		{name: "dark reply", env: map[string]string{"TERM": "xterm-256color"}, tty: true, reply: color.Black, want: true, wantQuery: true},
		{name: "no reply falls back to COLORFGBG", env: map[string]string{"TERM": "xterm", "COLORFGBG": "0;15"}, tty: true, replyErr: errors.New("timeout"), want: false, wantQuery: true},
		{name: "no reply and no COLORFGBG is dark", env: map[string]string{"TERM": "xterm"}, tty: true, replyErr: errors.New("timeout"), want: true, wantQuery: true},
		{name: "tmux is not queried", env: map[string]string{"TERM": "tmux-256color", "COLORFGBG": "0;15"}, tty: true, reply: color.Black, want: false},
		{name: "screen is not queried", env: map[string]string{"TERM": "screen"}, tty: true, reply: light, want: true},
		{name: "dumb is not queried", env: map[string]string{"TERM": "dumb"}, tty: true, reply: light, want: true},
		{name: "not a terminal", env: map[string]string{"TERM": "xterm", "COLORFGBG": "0;15"}, tty: false, reply: light, want: true},
		{name: "CI", env: map[string]string{"TERM": "xterm", "CI": "true"}, tty: true, reply: light, want: true},
		{name: "windows assumes dark", env: map[string]string{"TERM": "xterm", "COLORFGBG": "0;15"}, tty: true, windows: true, reply: light, want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prevQuery, prevTTY, prevWindows := queryBackground, stdoutIsTerminal, isWindows
			t.Cleanup(func() { queryBackground, stdoutIsTerminal, isWindows = prevQuery, prevTTY, prevWindows })
			queried := false
			queryBackground = func() (color.Color, error) {
				queried = true
				return c.reply, c.replyErr
			}
			stdoutIsTerminal = func() bool { return c.tty }
			isWindows = c.windows

			if got := detectDark(envOf(c.env)); got != c.want {
				t.Errorf("detectDark = %v, want %v", got, c.want)
			}
			if queried != c.wantQuery {
				t.Errorf("queried the terminal = %v, want %v", queried, c.wantQuery)
			}
		})
	}
}

func TestInitAutoUsesDetectedBackground(t *testing.T) {
	prevQuery, prevTTY := queryBackground, stdoutIsTerminal
	t.Cleanup(func() {
		queryBackground, stdoutIsTerminal = prevQuery, prevTTY
		resetTheme()
	})
	queryBackground = func() (color.Color, error) { return color.White, nil }
	stdoutIsTerminal = func() bool { return true }

	if err := Init(envOf(map[string]string{"TERM": "xterm-256color"})); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if active.dark {
		t.Error("a white background should select the light palette")
	}
	if Hint.GetForeground() != tokenMuted.on(false) {
		t.Errorf("Hint fg = %v, want the light muted token", Hint.GetForeground())
	}
}
