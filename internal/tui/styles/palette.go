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
// AA (4.5:1) on Solarized, Campbell, VS Code, Dracula, Gruvbox, black and
// white.
var (
	tokenMuted  = token{Light: "#5C5C5C", Dark: "#A8A8A8"}
	tokenAccent = token{Light: "#5B3CC4", Dark: "#A78BFA"}
	// tokenAccent2 reuses a pod colour instead of adding a hex value, so the
	// dashboard's charts and gradient end bring no colour the contrast
	// tests do not already hold.
	tokenAccent2 = podColors[accent2PodColor]
	tokenOK      = token{Light: "#1A7F37", Dark: "#04B575"}
	tokenWarn    = token{Light: "#8A5A00", Dark: "#F2A65A"}
	tokenErr     = token{Light: "#CF222E", Dark: "#FF7B72"}

	// The selected row and search matches paint their own background, so
	// one pair serves both themes.
	tokenSelectionFg = token{Light: "#FFFFFF", Dark: "#FFFFFF"}
	tokenSelectionBg = token{Light: "#7D56F4", Dark: "#7D56F4"}
	tokenMatchFg     = token{Light: "#000000", Dark: "#000000"}
	tokenMatchBg     = token{Light: "#FFD166", Dark: "#FFD166"}
)

// tokenSet is one preset's tokens: the default theme or high contrast.
// pods are in podColors' order, so a PodColorID names the same slot in
// every set.
type tokenSet struct {
	muted, accent, accent2   token
	ok, warn, err            token
	selectionFg, selectionBg token
	matchFg, matchBg         token
	pods                     []token
}

// defaultTokens are the tokens above: WCAG AA (4.5:1) for text.
var defaultTokens = tokenSet{
	muted:       tokenMuted,
	accent:      tokenAccent,
	accent2:     tokenAccent2,
	ok:          tokenOK,
	warn:        tokenWarn,
	err:         tokenErr,
	selectionFg: tokenSelectionFg,
	selectionBg: tokenSelectionBg,
	matchFg:     tokenMatchFg,
	matchBg:     tokenMatchBg,
	pods:        podColors,
}

// palette is what every style is built from: the tokens resolved for one
// background. In mono every colour is NoColor and the styles carry state
// through bold, underline and reverse.
type palette struct {
	muted, accent, accent2   color.Color
	ok, warn, err            color.Color
	selectionFg, selectionBg color.Color
	matchFg, matchBg         color.Color
	pods                     []color.Color
	dark                     bool
	mono                     bool
}

// resolve picks every token's variant for a dark or a light background.
func (s tokenSet) resolve(dark bool) palette {
	pods := make([]color.Color, len(s.pods))
	for i, t := range s.pods {
		pods[i] = t.on(dark)
	}
	return palette{
		muted:       s.muted.on(dark),
		accent:      s.accent.on(dark),
		accent2:     s.accent2.on(dark),
		ok:          s.ok.on(dark),
		warn:        s.warn.on(dark),
		err:         s.err.on(dark),
		selectionFg: s.selectionFg.on(dark),
		selectionBg: s.selectionBg.on(dark),
		matchFg:     s.matchFg.on(dark),
		matchBg:     s.matchBg.on(dark),
		pods:        pods,
		dark:        dark,
	}
}

// colourPalette is the default theme for a dark or a light background.
func colourPalette(dark bool) palette {
	return defaultTokens.resolve(dark)
}

// monoPalette has no colour at all. It counts as dark only so that
// anything still asking for a background gets a fixed answer.
var monoPalette = palette{
	muted:       lipgloss.NoColor{},
	accent:      lipgloss.NoColor{},
	accent2:     lipgloss.NoColor{},
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
