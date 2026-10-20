package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// token is a semantic colour: one value for light terminal backgrounds and
// one for dark ones. Values are hex on purpose: ANSI indexes follow the
// terminal's scheme, and Solarized paints bright black (8) in its own
// background colour.
type token struct {
	Light string
	Dark  string
}

// on returns the variant for a dark or a light background.
func (t token) on(dark bool) color.Color {
	return lipgloss.LightDark(dark)(lipgloss.Color(t.Light), lipgloss.Color(t.Dark))
}

// Semantic colour tokens. contrast_test.go holds every text token to WCAG
// AA (4.5:1) on Solarized, Campbell, Dracula, black and white.
var (
	tokenMuted  = token{Light: "#5C5C5C", Dark: "#A8A8A8"}
	tokenAccent = token{Light: "#5B3CC4", Dark: "#A78BFA"}
	tokenOK     = token{Light: "#1A7F37", Dark: "#04B575"}
	tokenWarn   = token{Light: "#8A5A00", Dark: "#F2A65A"}
	tokenErr    = token{Light: "#CF222E", Dark: "#FF7B72"}

	// The selected row and search matches paint their own background, so
	// one pair serves both themes.
	tokenSelectionFg = token{Light: "#FFFFFF", Dark: "#FFFFFF"}
	tokenSelectionBg = token{Light: "#7D56F4", Dark: "#7D56F4"}
	tokenMatchFg     = token{Light: "#000000", Dark: "#000000"}
	tokenMatchBg     = token{Light: "#FFD166", Dark: "#FFD166"}
)

// palette is what every style is built from: the tokens resolved for one
// background. In mono every colour is NoColor and the styles carry state
// through bold, underline and reverse.
type palette struct {
	muted, accent, ok, warn, err color.Color
	selectionFg, selectionBg     color.Color
	matchFg, matchBg             color.Color
	dark                         bool
	mono                         bool
}

// colourPalette resolves every token for a dark or a light background.
func colourPalette(dark bool) palette {
	return palette{
		muted:       tokenMuted.on(dark),
		accent:      tokenAccent.on(dark),
		ok:          tokenOK.on(dark),
		warn:        tokenWarn.on(dark),
		err:         tokenErr.on(dark),
		selectionFg: tokenSelectionFg.on(dark),
		selectionBg: tokenSelectionBg.on(dark),
		matchFg:     tokenMatchFg.on(dark),
		matchBg:     tokenMatchBg.on(dark),
		dark:        dark,
	}
}

// monoPalette has no colour at all. It counts as dark only so that
// anything still asking for a background gets a fixed answer.
var monoPalette = palette{
	muted:       lipgloss.NoColor{},
	accent:      lipgloss.NoColor{},
	ok:          lipgloss.NoColor{},
	warn:        lipgloss.NoColor{},
	err:         lipgloss.NoColor{},
	selectionFg: lipgloss.NoColor{},
	selectionBg: lipgloss.NoColor{},
	matchFg:     lipgloss.NoColor{},
	matchBg:     lipgloss.NoColor{},
	dark:        true,
	mono:        true,
}
