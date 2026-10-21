package dashboard

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// TestSeverityGlyphs is the one-glyph-per-severity rule: every severity
// draws its own glyph, in colour and mono alike.
func TestSeverityGlyphs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sev  dash.Severity
		want string
	}{
		{dash.SevOK, styles.GlyphOK},
		{dash.SevInfo, styles.GlyphInfo},
		{dash.SevMinor, styles.GlyphMinor},
		{dash.SevWaiting, styles.GlyphWarn},
		{dash.SevFailing, styles.GlyphErr},
	}
	for _, tt := range tests {
		if got := markGlyphs[markOf(tt.sev)]; got != tt.want {
			t.Errorf("severity %d draws %q, want %q", tt.sev, got, tt.want)
		}
	}
}

// TestHotMeterMatchesItsChip is the critique's check: a meter at 85% or
// more marks itself with the glyph of the severity its node and its
// verdict chip carry, △, never ▲.
func TestHotMeterMatchesItsChip(t *testing.T) {
	t.Parallel()

	if viz.HotGlyph != markGlyphs[markOf(dash.SevWaiting)] {
		t.Fatalf("hot meter glyph %q, chip glyph %q", viz.HotGlyph, markGlyphs[markOf(dash.SevWaiting)])
	}
	if viz.MeterLevel(85) != viz.LevelHot || viz.MeterLevel(84) == viz.LevelHot {
		t.Fatal("the meter's hot band must start at the data layer's 85%")
	}
}
