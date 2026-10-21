package styles

import "charm.land/lipgloss/v2"

// Dashboard glyphs. Every state is a glyph plus a word, and the glyph is
// what NO_COLOR and mono still show, so no state relies on colour.
const (
	// GlyphOK marks a healthy object.
	GlyphOK = "●"
	// GlyphErr marks a failing object.
	GlyphErr = "▲"
	// GlyphWarn marks an object that is waiting or degraded.
	GlyphWarn = "△"
	// GlyphOff marks something switched off or absent.
	GlyphOff = "○"
	// GlyphInfo marks information worth a look, such as a recent restart.
	GlyphInfo = "◇"
	// GlyphFact marks a neutral fact.
	GlyphFact = "◆"
	// GlyphMinor marks a minor state, such as an evicted pod.
	GlyphMinor = "·"
	// GlyphUnknown marks a value that is forbidden or unknown.
	GlyphUnknown = "—"
	// GlyphApprox marks a value k4s computes itself rather than reads from
	// a kubectl command.
	GlyphApprox = "≈"
	// CursorMarker leads the row under the cursor, so the cursor shows
	// even where reverse video does not.
	CursorMarker = "›"
)

// DashboardStyles are the dashboard's styles for one theme. It is a plain
// value: Dashboard hands every caller its own copy.
type DashboardStyles struct {
	// Panel is an unfocused panel: a rounded, muted border. Render the title
	// into the top edge with the border's MiddleRight and MiddleLeft.
	Panel lipgloss.Style
	// PanelFocused is the focused panel. Its heavy border marks focus in
	// every theme, mono included.
	PanelFocused lipgloss.Style
	// Title is an unfocused panel's "N name" title.
	Title lipgloss.Style
	// TitleFocused is the focused panel's title; bold, so mono shows focus
	// in the title as well as the border.
	TitleFocused lipgloss.Style
	// Label is muted text: column headers, border notes, legends and the
	// footer's question note.
	Label lipgloss.Style

	// OK styles GlyphOK.
	OK lipgloss.Style
	// Info styles GlyphInfo.
	Info lipgloss.Style
	// Minor styles GlyphMinor.
	Minor lipgloss.Style
	// Warn styles GlyphWarn; bold in mono.
	Warn lipgloss.Style
	// Err styles GlyphErr; bold, and underlined in mono.
	Err lipgloss.Style
	// Off styles GlyphOff.
	Off lipgloss.Style
	// Fact styles GlyphFact.
	Fact lipgloss.Style
	// Unknown styles GlyphUnknown.
	Unknown lipgloss.Style

	// Selection is the row under the cursor, the list views' selected row;
	// reverse video in mono. Lead the row with CursorMarker.
	Selection lipgloss.Style
	// Approx styles GlyphApprox and the history charts k4s builds itself.
	Approx lipgloss.Style
	// Heartbeat is the banner glyph that steps once per changed snapshot.
	Heartbeat lipgloss.Style
}

// dash is the dashboard styles of the active theme.
var dash DashboardStyles

// Dashboard returns the dashboard styles of the active theme. Like the
// package styles, read it when rendering: Init replaces it.
func Dashboard() DashboardStyles { return dash }

// Mono reports whether the active theme is mono, where gradients are off
// and state shows through glyphs, bold, underline and reverse alone.
func Mono() bool { return active.mono }

// dashboardStyles builds the dashboard styles from p. Nothing here may
// change a style's size or text between themes: colour output stripped of
// its escapes must equal mono output.
func dashboardStyles(p palette) DashboardStyles {
	plain := lipgloss.NewStyle()
	return DashboardStyles{
		Panel: plain.
			Border(lipgloss.RoundedBorder()).
			BorderForeground(p.muted),
		PanelFocused: plain.
			Border(lipgloss.ThickBorder()).
			BorderForeground(p.accent),
		Title:        plain.Foreground(p.accent),
		TitleFocused: plain.Foreground(p.accent).Bold(true),
		Label:        plain.Foreground(p.muted),
		OK:           plain.Foreground(p.ok),
		Info:         plain.Foreground(p.accent),
		Minor:        plain.Foreground(p.muted),
		Warn:         warnStyle(p),
		Err:          errStyle(p),
		Off:          plain.Foreground(p.muted),
		Fact:         plain.Foreground(p.accent),
		Unknown:      plain.Foreground(p.muted),
		Selection:    selectedStyle(p),
		Approx:       plain.Foreground(p.accent2),
		Heartbeat:    plain.Foreground(p.accent2),
	}
}
