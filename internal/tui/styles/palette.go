package styles

import "github.com/charmbracelet/lipgloss"

// Semantic colour tokens. Light variants are for light terminal
// backgrounds and Dark variants for dark ones; contrast_test.go holds
// every text token to WCAG AA (4.5:1) on Solarized, Campbell, Dracula,
// black and white. Values are hex on purpose: ANSI indexes follow the
// terminal's scheme, and Solarized paints bright black (8) in its own
// background colour.
var (
	tokenMuted  = lipgloss.AdaptiveColor{Light: "#5C5C5C", Dark: "#A8A8A8"}
	tokenAccent = lipgloss.AdaptiveColor{Light: "#5B3CC4", Dark: "#A78BFA"}
	tokenOK     = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#04B575"}
	tokenWarn   = lipgloss.AdaptiveColor{Light: "#8A5A00", Dark: "#F2A65A"}
	tokenErr    = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#FF7B72"}

	// The selected row and search matches paint their own background, so
	// one pair serves both themes.
	tokenSelectionFg = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}
	tokenSelectionBg = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#7D56F4"}
	tokenMatchFg     = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#000000"}
	tokenMatchBg     = lipgloss.AdaptiveColor{Light: "#FFD166", Dark: "#FFD166"}
)

// palette is what every style is built from. In mono every colour is
// NoColor and the styles carry state through bold, underline and reverse.
type palette struct {
	muted, accent, ok, warn, err lipgloss.TerminalColor
	selectionFg, selectionBg     lipgloss.TerminalColor
	matchFg, matchBg             lipgloss.TerminalColor
	mono                         bool
}

var colourPalette = palette{
	muted:       tokenMuted,
	accent:      tokenAccent,
	ok:          tokenOK,
	warn:        tokenWarn,
	err:         tokenErr,
	selectionFg: tokenSelectionFg,
	selectionBg: tokenSelectionBg,
	matchFg:     tokenMatchFg,
	matchBg:     tokenMatchBg,
}

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
	mono:        true,
}
