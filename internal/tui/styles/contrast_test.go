package styles

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// minTextContrast is the WCAG 2.x AA threshold for normal-size text.
const minTextContrast = 4.5

// Terminal backgrounds the palette must stay readable on. Issue #3 was
// filed against Solarized Dark, where the old hint grey measured 2.5:1.
var (
	darkBackgrounds = map[string]string{
		"solarized dark": "#002B36",
		"black":          "#000000",
		"campbell":       "#0C0C0C",
		"vscode dark":    "#1E1E1E",
		"dracula":        "#282A36",
	}
	lightBackgrounds = map[string]string{
		"solarized light": "#FDF6E3",
		"white":           "#FFFFFF",
	}
)

// textTokens lists every token drawn as text on the terminal's own
// background.
func textTokens() map[string]lipgloss.AdaptiveColor {
	out := map[string]lipgloss.AdaptiveColor{
		"muted":  tokenMuted,
		"accent": tokenAccent,
		"ok":     tokenOK,
		"warn":   tokenWarn,
		"err":    tokenErr,
	}
	for i, c := range podColors {
		out[fmt.Sprintf("pod %d", i)] = c
	}
	return out
}

// paintedPairs lists text tokens that bring their own background, so only
// the pair itself has to contrast.
func paintedPairs() map[string][2]lipgloss.AdaptiveColor {
	return map[string][2]lipgloss.AdaptiveColor{
		"selection": {tokenSelectionFg, tokenSelectionBg},
		"match":     {tokenMatchFg, tokenMatchBg},
	}
}

func TestTextTokensContrastOnDarkBackgrounds(t *testing.T) {
	t.Parallel()
	for name, tok := range textTokens() {
		for bgName, bg := range darkBackgrounds {
			t.Run(name+" on "+bgName, func(t *testing.T) {
				t.Parallel()
				assertContrast(t, tok.Dark, bg)
			})
		}
	}
}

func TestTextTokensContrastOnLightBackgrounds(t *testing.T) {
	t.Parallel()
	for name, tok := range textTokens() {
		for bgName, bg := range lightBackgrounds {
			t.Run(name+" on "+bgName, func(t *testing.T) {
				t.Parallel()
				assertContrast(t, tok.Light, bg)
			})
		}
	}
}

func TestPaintedPairsContrast(t *testing.T) {
	t.Parallel()
	for name, pair := range paintedPairs() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertContrast(t, pair[0].Dark, pair[1].Dark)
			assertContrast(t, pair[0].Light, pair[1].Light)
		})
	}
}

// ANSI indexes resolve through the terminal's own scheme (Solarized maps
// bright black to its background), so every token must be a literal hex.
func TestTokensAreHexNotPaletteIndexes(t *testing.T) {
	t.Parallel()
	all := textTokens()
	for name, pair := range paintedPairs() {
		all[name+" fg"] = pair[0]
		all[name+" bg"] = pair[1]
	}
	for name, tok := range all {
		for _, v := range []string{tok.Light, tok.Dark} {
			if _, err := luminance(v); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
}

func TestContrastRatioMatchesWCAGReferences(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b string
		want float64
	}{
		{name: "black on white", a: "#000000", b: "#FFFFFF", want: 21},
		{name: "symmetric", a: "#FFFFFF", b: "#000000", want: 21},
		{name: "same colour", a: "#7D56F4", b: "#7D56F4", want: 1},
		{name: "grey 777 on white", a: "#777777", b: "#FFFFFF", want: 4.48},
		{name: "selected row", a: "#FFFFFF", b: "#7D56F4", want: 4.66},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := contrastRatio(c.a, c.b)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(got-c.want) > 0.01 {
				t.Errorf("contrast(%s, %s) = %.3f, want %.2f", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestLuminanceRejectsNonHex(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"", "8", "240", "#FFF", "#GGGGGG", "7D56F4"} {
		if _, err := luminance(v); err == nil {
			t.Errorf("luminance(%q) should fail", v)
		}
	}
}

func assertContrast(t *testing.T, fg, bg string) {
	t.Helper()
	got, err := contrastRatio(fg, bg)
	if err != nil {
		t.Fatal(err)
	}
	if got < minTextContrast {
		t.Errorf("%s on %s = %.2f:1, want >= %.1f:1", fg, bg, got, minTextContrast)
	}
}

// contrastRatio is the WCAG 2.x contrast ratio of two #RRGGBB colours.
func contrastRatio(a, b string) (float64, error) {
	la, err := luminance(a)
	if err != nil {
		return 0, err
	}
	lb, err := luminance(b)
	if err != nil {
		return 0, err
	}
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05), nil
}

// luminance is the WCAG 2.x relative luminance of a #RRGGBB colour.
func luminance(hex string) (float64, error) {
	if len(hex) != 7 || hex[0] != '#' {
		return 0, fmt.Errorf("parse colour %q: want #RRGGBB", hex)
	}
	v, err := strconv.ParseUint(hex[1:], 16, 32)
	if err != nil {
		return 0, fmt.Errorf("parse colour %q: %w", hex, err)
	}
	r := channel(v >> 16 & 0xFF)
	g := channel(v >> 8 & 0xFF)
	b := channel(v & 0xFF)
	return 0.2126*r + 0.7152*g + 0.0722*b, nil
}

func channel(v uint64) float64 {
	c := float64(v) / 255
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
