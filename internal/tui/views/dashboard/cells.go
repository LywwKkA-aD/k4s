package dashboard

import (
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// frame is what one render reads: the data, the cursor, and the styles of
// the active theme, taken once per View.
type frame struct {
	snap   dash.Snapshot
	loaded bool
	hist   history
	focus  focus
	since  time.Time
	ctx    string // kubeconfig context name, cleaned
	hint   string // "watch 5s", "paused", "refreshing…"
	th     viz.Theme
	ds     styles.DashboardStyles
}

// themeOf maps the dashboard styles onto the widgets' theme. The mono
// attributes ride along: warn is bold, err bold and underlined.
func themeOf(ds styles.DashboardStyles) viz.Theme {
	return viz.Theme{Muted: ds.Label, Accent: ds.Title, Accent2: ds.Approx, OK: ds.OK, Warn: ds.Warn, Err: ds.Err}
}

// mark is a state glyph. Every state is a glyph plus a word, and the glyph
// is what mono and NO_COLOR still show.
type mark int

const (
	markNone mark = iota
	markOK
	markErr
	markWarn
	markOff
	markInfo
	markFact
	markMinor
	markUnknown
)

var markGlyphs = [...]string{
	markNone:    " ",
	markOK:      styles.GlyphOK,
	markErr:     styles.GlyphErr,
	markWarn:    styles.GlyphWarn,
	markOff:     styles.GlyphOff,
	markInfo:    styles.GlyphInfo,
	markFact:    styles.GlyphFact,
	markMinor:   styles.GlyphMinor,
	markUnknown: styles.GlyphUnknown,
}

// markOf is the glyph of a severity.
func markOf(s dash.Severity) mark {
	switch s {
	case dash.SevFailing:
		return markErr
	case dash.SevWaiting:
		return markWarn
	case dash.SevMinor:
		return markMinor
	case dash.SevInfo:
		return markInfo
	}
	return markOK
}

// paint renders the glyph of k in its style.
func (f frame) paint(k mark) string {
	g := markGlyphs[k]
	switch k {
	case markOK:
		return f.ds.OK.Render(g)
	case markErr:
		return f.ds.Err.Render(g)
	case markWarn:
		return f.ds.Warn.Render(g)
	case markOff:
		return f.ds.Off.Render(g)
	case markInfo:
		return f.ds.Info.Render(g)
	case markFact:
		return f.ds.Fact.Render(g)
	case markMinor:
		return f.ds.Minor.Render(g)
	case markUnknown:
		return f.ds.Unknown.Render(g)
	}
	return g
}

// unreadable says why a source shows no data, never as a 0: denied,
// still to come, not served, and so on.
func (f frame) unreadable(what string, a dash.Avail) string {
	switch a.State {
	case dash.StateForbidden:
		return f.paint(markUnknown) + " " + what + " forbidden"
	case dash.StateUnknown:
		return f.paint(markFact) + " waiting for " + what
	case dash.StateAbsent:
		return f.paint(markOff) + " " + what + " not served"
	case dash.StateNotReady:
		return f.paint(markOff) + " " + what + " not ready"
	case dash.StateUnauthorized:
		return f.paint(markUnknown) + " " + what + ": credentials expired"
	case dash.StateThrottled:
		return f.paint(markWarn) + " " + what + " throttled"
	case dash.StateUnsupported:
		return f.paint(markUnknown) + " " + what + " not supported by this server"
	}
	return f.paint(markUnknown) + " " + what + " failed"
}

// muted renders text in the label style, and nothing for empty text.
func (f frame) muted(s string) string {
	if s == "" {
		return ""
	}
	return f.ds.Label.Render(s)
}

// fitText cuts s to w columns, ending in an ellipsis when cut, and pads it
// to exactly w.
func fitText(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return padRight(cutText(s, w), w)
}

// cutText shortens s to at most w columns with an ellipsis.
func cutText(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

func padRight(s string, w int) string {
	if gap := w - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// spread puts left and right on one line w wide, right-aligned, cutting
// left first; right alone when there is no room for both.
func spread(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if rw+2 > w {
		return fitText(right, w)
	}
	return padRight(cutText(left, w-rw-2), w-rw) + right
}

// ago is a coarsened duration as the panels print it: "<1m", "3h".
func ago(d time.Duration) string { return dash.FormatAgo(d) }
