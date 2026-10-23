package dashboard

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// rootMark is the glyph the root leads a status message of each level
// with (its statusMark): the status row and the dashboard share
// styles.GlyphOK, GlyphWarn and GlyphErr.
var rootMark = map[status.Level]string{status.OK: styles.GlyphOK, status.Warn: styles.GlyphWarn, status.Error: styles.GlyphErr}

// TestToastInTheStatusRowHasOneGlyph: outside the wide layout a toast is
// a status message, and the row shows one glyph for it, the change's
// severity's: the root's for the levels that have one, the dashboard's
// otherwise. A failing change never shows the warn glyph, and never goes
// as an error, which would stay up until a key.
func TestToastInTheStatusRowHasOneGlyph(t *testing.T) {
	t.Parallel()

	obj := dash.ObjectKey{Kind: "pod", Namespace: "shop", Name: "api"}
	const rest = "pod shop/api → x · +2"
	for _, sev := range []dash.Severity{dash.SevOK, dash.SevInfo, dash.SevMinor, dash.SevWaiting, dash.SevFailing} {
		level, text := toast{c: change{sev: sev, obj: obj, to: "x"}, more: 2}.status()
		row := rootMark[level]
		if row != "" {
			row += " "
		}
		row += text
		want := markGlyphs[markOf(sev)] + " " + rest
		if row != want || level == status.Error {
			t.Errorf("severity %d: level %d, status row %q; want %q", sev, level, row, want)
		}
	}
}
