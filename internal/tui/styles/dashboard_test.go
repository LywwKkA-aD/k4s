package styles

import (
	"image/color"
	"slices"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// The spec allows no new hex values: accent2 is the cyan pods already use.
func TestAccent2IsPodColourEight(t *testing.T) {
	t.Parallel()
	if tokenAccent2 != podColors[8] {
		t.Fatalf("accent2 = %+v, want pod colour 8 %+v", tokenAccent2, podColors[8])
	}
	cases := []struct {
		name string
		p    palette
		want color.Color
	}{
		{name: "dark", p: colourPalette(true), want: lipgloss.Color(podColors[8].Dark)},
		{name: "light", p: colourPalette(false), want: lipgloss.Color(podColors[8].Light)},
		{name: "mono", p: monoPalette, want: lipgloss.NoColor{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if c.p.accent2 != c.want {
				t.Errorf("accent2 = %v, want %v", c.p.accent2, c.want)
			}
		})
	}
}

// Glyphs sit in fixed one-cell columns; a two-cell glyph would shift every
// row after it.
func TestDashboardGlyphsAreOneCellAndDistinct(t *testing.T) {
	t.Parallel()
	glyphs := []string{
		GlyphOK, GlyphErr, GlyphWarn, GlyphOff, GlyphInfo, GlyphFact,
		GlyphMinor, GlyphUnknown, GlyphApprox, CursorMarker,
	}
	seen := make(map[string]bool, len(glyphs))
	for _, g := range glyphs {
		if w := ansi.StringWidth(g); w != 1 {
			t.Errorf("glyph %q is %d cells wide, want 1", g, w)
		}
		if seen[g] {
			t.Errorf("glyph %q is used twice", g)
		}
		seen[g] = true
	}
	want := map[string]string{
		GlyphOK: "●", GlyphErr: "▲", GlyphWarn: "△", GlyphOff: "○", GlyphInfo: "◇",
		GlyphFact: "◆", GlyphMinor: "·", GlyphUnknown: "—", GlyphApprox: "≈", CursorMarker: "›",
	}
	for got, w := range want {
		if got != w {
			t.Errorf("glyph %q, want %q", got, w)
		}
	}
}

// A heavy border marks focus in every theme, so focus never depends on
// colour.
func TestDashboardPanelBorders(t *testing.T) {
	t.Parallel()
	for _, p := range []palette{colourPalette(true), colourPalette(false), monoPalette} {
		d := dashboardStyles(p)
		if got := d.Panel.GetBorderStyle(); got != lipgloss.RoundedBorder() {
			t.Errorf("mono=%v dark=%v: panel border = %+v, want rounded", p.mono, p.dark, got)
		}
		if got := d.PanelFocused.GetBorderStyle(); got != lipgloss.ThickBorder() {
			t.Errorf("mono=%v dark=%v: focused border = %+v, want thick", p.mono, p.dark, got)
		}
		if got := d.Panel.GetBorderTopForeground(); got != p.muted {
			t.Errorf("mono=%v dark=%v: panel border colour = %v, want muted", p.mono, p.dark, got)
		}
		if got := d.PanelFocused.GetBorderTopForeground(); got != p.accent {
			t.Errorf("mono=%v dark=%v: focused border colour = %v, want accent", p.mono, p.dark, got)
		}
		if !d.TitleFocused.GetBold() {
			t.Errorf("mono=%v dark=%v: focused title is not bold", p.mono, p.dark)
		}
		if d.Title.GetBold() {
			t.Errorf("mono=%v dark=%v: unfocused title is bold, so focus does not show", p.mono, p.dark)
		}
	}
}

// Each style takes the token the spec's colour table assigns it.
func TestDashboardStyleColours(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{true, false} {
		p := colourPalette(dark)
		d := dashboardStyles(p)
		cases := []struct {
			name  string
			style lipgloss.Style
			want  color.Color
		}{
			{name: "title", style: d.Title, want: p.accent},
			{name: "title focused", style: d.TitleFocused, want: p.accent},
			{name: "label", style: d.Label, want: p.muted},
			{name: "ok", style: d.OK, want: p.ok},
			{name: "info", style: d.Info, want: p.accent},
			{name: "minor", style: d.Minor, want: p.muted},
			{name: "warn", style: d.Warn, want: p.warn},
			{name: "err", style: d.Err, want: p.err},
			{name: "off", style: d.Off, want: p.muted},
			{name: "fact", style: d.Fact, want: p.accent},
			{name: "unknown", style: d.Unknown, want: p.muted},
			{name: "approx", style: d.Approx, want: p.accent2},
			{name: "heartbeat", style: d.Heartbeat, want: p.accent2},
			{name: "selection", style: d.Selection, want: p.selectionFg},
		}
		for _, c := range cases {
			if got := c.style.GetForeground(); got != c.want {
				t.Errorf("dark=%v %s: fg = %v, want %v", dark, c.name, got, c.want)
			}
		}
		if got := d.Selection.GetBackground(); got != p.selectionBg {
			t.Errorf("dark=%v selection: bg = %v, want %v", dark, got, p.selectionBg)
		}
	}
}

// The dashboard selection is the list views' selected row, so the cursor
// looks the same everywhere.
func TestDashboardSelectionMatchesTable(t *testing.T) {
	t.Cleanup(resetTheme)
	for _, p := range []palette{colourPalette(true), colourPalette(false), monoPalette} {
		use(p)
		if got, want := Dashboard().Selection.Render("row"), Table().Selected.Render("row"); got != want {
			t.Errorf("mono=%v dark=%v: selection %q, table selected %q", p.mono, p.dark, got, want)
		}
	}
}

// Under NO_COLOR every state still reads from glyphs and attributes, so
// stripping the escapes from colour output must give the mono output.
func TestDashboardColourStripsToMono(t *testing.T) {
	t.Parallel()
	mono := dashboardSample(dashboardStyles(monoPalette))
	for _, dark := range []bool{true, false} {
		for name, out := range dashboardSample(dashboardStyles(colourPalette(dark))) {
			if got, want := ansi.Strip(out), ansi.Strip(mono[name]); got != want {
				t.Errorf("dark=%v %s: colour strips to %q, mono to %q", dark, name, got, want)
			}
		}
	}
}

// In mono only the states that need attention, the focused title and the
// cursor carry attributes; the rest stay plain so they do not compete.
func TestDashboardMonoQuietStylesArePlain(t *testing.T) {
	t.Parallel()
	out := dashboardSample(dashboardStyles(monoPalette))
	profile := monoProfile(true, "xterm-256color")
	for _, name := range []string{
		"dash panel", "dash title", "dash label", "dash ok", "dash info",
		"dash minor", "dash off", "dash fact", "dash unknown", "dash approx", "dash heartbeat",
	} {
		codes := sgrCodes(shown(t, profile, out[name]))
		if slices.ContainsFunc(codes, func(c int) bool { return c == 1 || c == 4 || c == 7 }) {
			t.Errorf("%s: mono adds attributes %v", name, codes)
		}
	}
}

func TestDashboardFollowsTheme(t *testing.T) {
	t.Cleanup(resetTheme)
	cases := []struct {
		name     string
		p        palette
		wantMono bool
	}{
		{name: "dark", p: colourPalette(true)},
		{name: "light", p: colourPalette(false)},
		{name: "mono", p: monoPalette, wantMono: true},
	}
	for _, c := range cases {
		use(c.p)
		if got := Mono(); got != c.wantMono {
			t.Errorf("%s: Mono() = %v, want %v", c.name, got, c.wantMono)
		}
		if got := Dashboard().Approx.GetForeground(); got != c.p.accent2 {
			t.Errorf("%s: Approx fg = %v, want %v", c.name, got, c.p.accent2)
		}
	}
}

// dashboardSample renders every dashboard style, keyed as sample() keys
// them.
func dashboardSample(d DashboardStyles) map[string]string {
	return map[string]string{
		"dash panel":         d.Panel.Render("body"),
		"dash panel focused": d.PanelFocused.Render("body"),
		"dash title":         d.Title.Render(" 1 cluster "),
		"dash title focused": d.TitleFocused.Render(" 3 pods "),
		"dash label":         d.Label.Render("NAMESPACE"),
		"dash ok":            d.OK.Render(GlyphOK),
		"dash info":          d.Info.Render(GlyphInfo),
		"dash minor":         d.Minor.Render(GlyphMinor),
		"dash warn":          d.Warn.Render(GlyphWarn),
		"dash err":           d.Err.Render(GlyphErr),
		"dash off":           d.Off.Render(GlyphOff),
		"dash fact":          d.Fact.Render(GlyphFact),
		"dash unknown":       d.Unknown.Render(GlyphUnknown),
		"dash selection":     d.Selection.Render(CursorMarker + GlyphErr + " shop"),
		"dash approx":        d.Approx.Render(GlyphApprox + "5m"),
		"dash heartbeat":     d.Heartbeat.Render("⣽"),
	}
}

// The profile a truecolor terminal uses must keep the token colours as
// they are, or the contrast tests would measure something else.
func TestDashboardTruecolorKeepsTokens(t *testing.T) {
	t.Parallel()
	d := dashboardStyles(colourPalette(true))
	got := shown(t, colorprofile.TrueColor, d.Approx.Render("≈"))
	if want := "38;2;63;193;201"; !containsSGR(got, want) {
		t.Errorf("approx = %q, want SGR %s (accent2 #3FC1C9)", got, want)
	}
}

func containsSGR(s, params string) bool {
	for _, m := range sgrPattern.FindAllStringSubmatch(s, -1) {
		if m[1] == params {
			return true
		}
	}
	return false
}

// The meter's hot mark and the warn chip are one glyph, so they can never
// disagree. viz does not import styles, so the check lives here.
func TestMeterHotGlyphIsTheWarnGlyph(t *testing.T) {
	t.Parallel()
	if viz.HotGlyph != GlyphWarn {
		t.Errorf("viz.HotGlyph = %q, GlyphWarn = %q", viz.HotGlyph, GlyphWarn)
	}
	got := ansi.Strip(viz.Meter(viz.Mono(), 18, 90, 1))
	if !strings.HasSuffix(got, GlyphWarn) {
		t.Errorf("hot meter %q does not end in %q", got, GlyphWarn)
	}
}
