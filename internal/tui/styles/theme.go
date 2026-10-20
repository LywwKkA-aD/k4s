package styles

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// stdoutIsTerminal is a variable so tests can pretend to be on a terminal.
var stdoutIsTerminal = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

// Mode is a K4S_THEME value.
type Mode string

// Theme modes accepted in K4S_THEME.
const (
	// ModeAuto asks the terminal for its background colour.
	ModeAuto Mode = "auto"
	// ModeDark assumes a dark background without asking.
	ModeDark Mode = "dark"
	// ModeLight assumes a light background without asking.
	ModeLight Mode = "light"
	// ModeMono drops colour and marks state with bold, underline and
	// reverse video.
	ModeMono Mode = "mono"
)

// ErrInvalidTheme reports a K4S_THEME value k4s does not know.
var ErrInvalidTheme = errors.New("invalid K4S_THEME")

// ParseMode reads K4S_THEME and NO_COLOR through getenv. An empty
// K4S_THEME means auto, case and surrounding spaces are ignored, and a
// non-empty NO_COLOR (https://no-color.org) selects mono whatever the
// theme says. An unknown theme is an error even when NO_COLOR is set, so
// a typo never goes unnoticed.
func ParseMode(getenv func(string) string) (Mode, error) {
	raw := getenv("K4S_THEME")
	mode := Mode(strings.ToLower(strings.TrimSpace(raw)))
	if mode == "" {
		mode = ModeAuto
	}
	switch mode {
	case ModeAuto, ModeDark, ModeLight, ModeMono:
	default:
		return "", fmt.Errorf("%w %q: want one of %s, %s, %s, %s",
			ErrInvalidTheme, raw, ModeAuto, ModeDark, ModeLight, ModeMono)
	}
	if getenv("NO_COLOR") != "" {
		return ModeMono, nil
	}
	return mode, nil
}

// Init applies the theme chosen by K4S_THEME and NO_COLOR (see ParseMode)
// to every style in this package.
//
// Call it once from main, before building the TUI model and before
// tea.NewProgram. Auto mode asks the terminal for its background with an
// OSC 11 query; run any later and the reply races Bubble Tea's stdin
// reader. The other modes fix the answer so nothing queries at all.
func Init(getenv func(string) string) error {
	mode, err := ParseMode(getenv)
	if err != nil {
		return err
	}
	apply(mode, getenv)
	return nil
}

// forcedProfile is the colour profile mono pins the program to; nil lets
// Bubble Tea detect it.
var forcedProfile *colorprofile.Profile

func apply(mode Mode, getenv func(string) string) {
	forcedProfile = nil
	switch mode {
	case ModeDark:
		use(colourPalette(true))
	case ModeLight:
		use(colourPalette(false))
	case ModeMono:
		use(monoPalette)
		p := monoProfile(stdoutIsTerminal(), getenv("TERM"))
		forcedProfile = &p
	default:
		use(colourPalette(detectDark(getenv)))
	}
}

// ColorProfile returns the colour profile the program must render with, and
// false when Bubble Tea should detect it from the terminal. main passes it
// to tea.WithColorProfile.
func ColorProfile() (colorprofile.Profile, bool) {
	if forcedProfile == nil {
		return colorprofile.Unknown, false
	}
	return *forcedProfile, true
}

// monoProfile keeps text attributes on a real terminal and drops every
// escape code where they would be noise: pipes, files and TERM=dumb. The
// ASCII profile drops colour but keeps bold, underline and reverse, so the
// selected row stays visible whatever profile NO_COLOR would select.
func monoProfile(isTerminal bool, termName string) colorprofile.Profile {
	if !isTerminal || termName == "dumb" {
		return colorprofile.NoTTY
	}
	return colorprofile.ASCII
}
