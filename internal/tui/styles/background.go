package styles

import (
	"image/color"
	"strconv"
	"strings"
)

// backgroundState is how the theme tracks the terminal's background.
type backgroundState struct {
	// follow is set for auto and high-contrast, which re-theme when the
	// terminal answers; tokens is their preset.
	follow bool
	tokens tokenSet
	// ask reports whether the program may ask the terminal at all.
	ask bool
}

// background is what Init chose; SetBackground reads it.
var background backgroundState

// follow starts tokens on the guessed background and lets SetBackground
// move it once the terminal answers.
func follow(tokens tokenSet, getenv func(string) string) {
	background = backgroundState{follow: true, tokens: tokens, ask: canAskBackground(getenv)}
	use(tokens.resolve(guessDark(getenv)))
}

// WantsBackground reports whether the program should ask the terminal for
// its background (tea.RequestBackgroundColor) and hand the answer to
// SetBackground: the theme follows the background and the terminal can be
// asked.
func WantsBackground() bool {
	return background.follow && background.ask
}

// SetBackground re-themes for the terminal's background bg when the theme
// follows it (auto, high-contrast) and bg moves it between dark and light.
// It reports whether the styles changed. Call it where views render (the
// program's Update), since it replaces every style in this package.
func SetBackground(bg color.Color) bool {
	if !background.follow || bg == nil {
		return false
	}
	dark := isDarkColor(bg)
	if dark == active.dark {
		return false
	}
	use(background.tokens.resolve(dark))
	return true
}

// guessDark is the background before the terminal answers: COLORFGBG on a
// terminal, else dark. CI and a stdout that is not a terminal assume dark.
func guessDark(getenv func(string) string) bool {
	if getenv("CI") != "" || !stdoutIsTerminal() {
		return true
	}
	return colorFGBGIsDark(getenv("COLORFGBG"))
}

// canAskBackground reports whether an OSC 11 answer would describe this
// terminal. CI and a stdout that is not a terminal have none to give, and
// screen, tmux and dumb TERMs are never asked, since a multiplexer may sit
// in front of several terminals at once.
func canAskBackground(getenv func(string) string) bool {
	return getenv("CI") == "" && stdoutIsTerminal() && !skipsBackgroundQuery(getenv("TERM"))
}

func skipsBackgroundQuery(termName string) bool {
	for _, prefix := range []string{"screen", "tmux", "dumb"} {
		if strings.HasPrefix(termName, prefix) {
			return true
		}
	}
	return false
}

// isDarkColor reports an HSL lightness below one half.
func isDarkColor(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	hi := max(r, g, b)
	lo := min(r, g, b)
	return float64(hi+lo)/2/0xFFFF < 0.5
}

// colorFGBGIsDark reads the background index from COLORFGBG ("fg;bg", or
// "fg;default;bg" in rxvt). A value it cannot read counts as dark.
func colorFGBGIsDark(v string) bool {
	if !strings.Contains(v, ";") {
		return true
	}
	parts := strings.Split(v, ";")
	i, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return true
	}
	c, ok := ansiIndexColor(i)
	if !ok {
		return true
	}
	return isDarkColor(c)
}

// ansiBasic are the 16 basic ANSI colours as xterm defines them.
var ansiBasic = [16]color.RGBA{
	{0x00, 0x00, 0x00, 0xFF}, {0x80, 0x00, 0x00, 0xFF}, {0x00, 0x80, 0x00, 0xFF}, {0x80, 0x80, 0x00, 0xFF},
	{0x00, 0x00, 0x80, 0xFF}, {0x80, 0x00, 0x80, 0xFF}, {0x00, 0x80, 0x80, 0xFF}, {0xC0, 0xC0, 0xC0, 0xFF},
	{0x80, 0x80, 0x80, 0xFF}, {0xFF, 0x00, 0x00, 0xFF}, {0x00, 0xFF, 0x00, 0xFF}, {0xFF, 0xFF, 0x00, 0xFF},
	{0x00, 0x00, 0xFF, 0xFF}, {0xFF, 0x00, 0xFF, 0xFF}, {0x00, 0xFF, 0xFF, 0xFF}, {0xFF, 0xFF, 0xFF, 0xFF},
}

// cubeLevels are the channel values of xterm's 6x6x6 colour cube.
var cubeLevels = [6]uint8{0x00, 0x5F, 0x87, 0xAF, 0xD7, 0xFF}

// ansiIndexColor returns xterm's colour for a 256-colour index.
func ansiIndexColor(i int) (color.RGBA, bool) {
	switch {
	case i < 0 || i > 255:
		return color.RGBA{}, false
	case i < 16:
		return ansiBasic[i], true
	case i < 232:
		n := i - 16
		return color.RGBA{cubeLevels[n/36], cubeLevels[n/6%6], cubeLevels[n%6], 0xFF}, true
	}
	v := uint8(8 + 10*(i-232)) //nolint:gosec // i is 232..255, v stays under 240
	return color.RGBA{v, v, v, 0xFF}, true
}
