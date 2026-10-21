package styles

import (
	"fmt"
	"image/color"
	"testing"

	"charm.land/lipgloss/v2"
)

// The high-contrast preset holds text to WCAG AAA and borders to the WCAG
// threshold for non-text contrast, on the same backgrounds as the default
// theme (Dracula and Gruvbox included).
const (
	minHighContrastText = 7.0
	minBorderContrast   = 3.0
)

func hcTextTokens() map[string]token {
	s := highContrastTokens
	out := map[string]token{
		"muted":   s.muted,
		"accent":  s.accent,
		"accent2": s.accent2,
		"ok":      s.ok,
		"warn":    s.warn,
		"err":     s.err,
	}
	for i, c := range s.pods {
		out[fmt.Sprintf("pod %d", i)] = c
	}
	return out
}

func assertContrastAtLeast(t *testing.T, fg, bg string, minRatio float64) {
	t.Helper()
	got, err := contrastRatio(fg, bg)
	if err != nil {
		t.Fatal(err)
	}
	if got < minRatio {
		t.Errorf("%s on %s = %.2f:1, want >= %.1f:1", fg, bg, got, minRatio)
	}
}

func TestHighContrastTextOnEveryBackground(t *testing.T) {
	t.Parallel()
	for name, tok := range hcTextTokens() {
		for bgName, bg := range darkBackgrounds {
			t.Run(name+" on "+bgName, func(t *testing.T) {
				t.Parallel()
				assertContrastAtLeast(t, tok.Dark, bg, minHighContrastText)
			})
		}
		for bgName, bg := range lightBackgrounds {
			t.Run(name+" on "+bgName, func(t *testing.T) {
				t.Parallel()
				assertContrastAtLeast(t, tok.Light, bg, minHighContrastText)
			})
		}
	}
}

func TestHighContrastPaintedPairs(t *testing.T) {
	t.Parallel()
	s := highContrastTokens
	pairs := map[string][2]token{
		"selection": {s.selectionFg, s.selectionBg},
		"match":     {s.matchFg, s.matchBg},
	}
	for name, pair := range pairs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertContrastAtLeast(t, pair[0].Dark, pair[1].Dark, minHighContrastText)
			assertContrastAtLeast(t, pair[0].Light, pair[1].Light, minHighContrastText)
		})
	}
}

// The gradient paints text, so its stops are text too.
func TestHighContrastGradientStops(t *testing.T) {
	t.Parallel()
	for _, theme := range []struct {
		name string
		dark bool
		bgs  map[string]string
	}{
		{name: "dark", dark: true, bgs: darkBackgrounds},
		{name: "light", dark: false, bgs: lightBackgrounds},
	} {
		for i, stop := range blendStops(highContrastPalette(theme.dark)) {
			for bgName, bg := range theme.bgs {
				t.Run(fmt.Sprintf("%s stop %d on %s", theme.name, i, bgName), func(t *testing.T) {
					t.Parallel()
					assertContrastAtLeast(t, hexOf(stop), bg, minHighContrastText)
				})
			}
		}
	}
}

func TestHighContrastTokensAreHex(t *testing.T) {
	t.Parallel()
	s := highContrastTokens
	all := hcTextTokens()
	for name, tok := range map[string]token{
		"selection fg": s.selectionFg, "selection bg": s.selectionBg,
		"match fg": s.matchFg, "match bg": s.matchBg,
	} {
		all[name] = tok
	}
	for name, tok := range all {
		for _, v := range []string{tok.Light, tok.Dark} {
			if _, err := luminance(v); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
	}
	if len(s.pods) != len(podColors) {
		t.Errorf("high contrast has %d pod colours, want %d like the default theme", len(s.pods), len(podColors))
	}
}

// borders returns every border colour the styles draw.
func borders() map[string]color.Color {
	d := Dashboard()
	return map[string]color.Color{
		"header":             Header.GetBorderBottomForeground(),
		"footer":             Footer.GetBorderTopForeground(),
		"stat":               Stat.GetBorderTopForeground(),
		"popup":              PopupBox.GetBorderTopForeground(),
		"table header":       Table().Header.GetBorderBottomForeground(),
		"dash panel":         d.Panel.GetBorderTopForeground(),
		"dash panel focused": d.PanelFocused.GetBorderTopForeground(),
	}
}

// The styles are package state, so this test runs sequentially.
func TestHighContrastBorders(t *testing.T) {
	t.Cleanup(resetTheme)
	for _, theme := range []struct {
		name string
		dark bool
		bgs  map[string]string
	}{
		{name: "dark", dark: true, bgs: darkBackgrounds},
		{name: "light", dark: false, bgs: lightBackgrounds},
	} {
		use(highContrastPalette(theme.dark))
		for name, c := range borders() {
			for bgName, bg := range theme.bgs {
				t.Run(fmt.Sprintf("%s %s on %s", theme.name, name, bgName), func(t *testing.T) {
					assertContrastAtLeast(t, hexOf(c), bg, minBorderContrast)
				})
			}
		}
	}
}

// Pods keep their colour slot under high contrast: the same ID resolves to
// the high-contrast variant of the same hue.
func TestHighContrastPodColours(t *testing.T) {
	t.Cleanup(resetTheme)
	for _, dark := range []bool{true, false} {
		use(highContrastPalette(dark))
		for i, id := range PodPalette() {
			want := highContrastTokens.pods[i].on(dark)
			if got := PodColor(id); got != want {
				t.Errorf("dark=%v pod %d = %v, want %v", dark, i, got, want)
			}
		}
	}
	use(highContrastPalette(true))
	if got := PodColor("#123456"); got != lipgloss.Color("#123456") {
		t.Errorf("an unknown id = %v, want it read as a colour", got)
	}
}
