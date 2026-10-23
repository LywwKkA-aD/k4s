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
var rootMark = map[status.Level]string{
	status.OK: styles.GlyphOK, status.Warn: styles.GlyphWarn, status.Error: styles.GlyphErr, status.Alert: styles.GlyphErr,
}

// TestToastInTheStatusRowHasOneGlyph: outside the wide layout a toast is
// a status message whose level carries the change's severity, and the
// row shows the root's one glyph for it. A failing change goes as an
// alert, coloured as an error but gone at the TTL, never as an error,
// which would stay up until a key. The toast adds no glyph of its own.
func TestToastInTheStatusRowHasOneGlyph(t *testing.T) {
	t.Parallel()

	obj := dash.ObjectKey{Kind: "pod", Namespace: "shop", Name: "api"}
	const rest = "pod shop/api → x · +2"
	cases := []struct {
		name  string
		sev   dash.Severity
		level status.Level
		row   string
	}{
		{name: "a recovery", sev: dash.SevOK, level: status.OK, row: styles.GlyphOK + " " + rest},
		{name: "information", sev: dash.SevInfo, level: status.Info, row: rest},
		{name: "a minor change", sev: dash.SevMinor, level: status.Info, row: rest},
		{name: "a change to waiting", sev: dash.SevWaiting, level: status.Warn, row: styles.GlyphWarn + " " + rest},
		{name: "a change to failing", sev: dash.SevFailing, level: status.Alert, row: styles.GlyphErr + " " + rest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			level, text := toast{c: change{sev: tc.sev, obj: obj, to: "x"}, more: 2}.status()
			row := text
			if mark := rootMark[level]; mark != "" {
				row = mark + " " + text
			}
			if level != tc.level || row != tc.row {
				t.Fatalf("level %d, status row %q; want %d, %q", level, row, tc.level, tc.row)
			}
		})
	}
}
