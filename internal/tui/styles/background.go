package styles

import (
	"image/color"
	"os"
	"runtime"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// queryBackground asks the terminal for its background colour with an OSC
// 11 query. It writes to and reads from stdout, as k4s always has, so a
// redirected stdin does not change the answer. Tests replace it so they
// never touch the real terminal.
var queryBackground = func() (color.Color, error) {
	return lipgloss.BackgroundColor(os.Stdout, os.Stdout)
}

// isWindows is a variable so tests can take the Windows branch anywhere.
var isWindows = runtime.GOOS == "windows"

// detectDark reports whether the terminal background is dark. Windows, CI
// and a stdout that is not a terminal assume dark without asking. TERM
// values for screen, tmux and dumb are never queried, since a multiplexer
// may sit in front of several terminals at once. Without an answer,
// COLORFGBG decides, and then dark.
func detectDark(getenv func(string) string) bool {
	if isWindows || getenv("CI") != "" || !stdoutIsTerminal() {
		return true
	}
	if !skipsBackgroundQuery(getenv("TERM")) {
		if bg, err := queryBackground(); err == nil && bg != nil {
			return isDarkColor(bg)
		}
	}
	return colorFGBGIsDark(getenv("COLORFGBG"))
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
