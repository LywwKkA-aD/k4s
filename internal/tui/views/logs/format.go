package logs

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// noCurrent is the current-match index of a line the match n/N stands on
// is not in.
const noCurrent = -1

// formatRawLine turns a raw entry into the string the viewport renders.
//
// Modes:
//   - !multi: no prefix (single-pod view; pod name is in the title).
//   - multi && !full: a coloured "▌" bar — minimal but tells replicas apart.
//   - multi && full: the canonical "[pod-name] line" prefix.
//
// Stream-error and stream-closed notices keep the pod name inline so they
// remain readable regardless of the prefix mode.
func formatRawLine(ln logLine, multi, full bool, colour styles.PodColorID) string {
	return formatLine(ln, multi, full, colour, nil, noCurrent)
}

// formatLine is formatRawLine with the occurrences in spans painted as
// search matches, current (an index into spans, or noCurrent) as the one
// n/N stands on. Only the text is painted, segment by segment, so a match
// can never land inside a colour code. The text always ends the line,
// which is how a match's column is found again.
func formatLine(ln logLine, multi, full bool, colour styles.PodColorID, spans []span, current int) string {
	switch ln.kind {
	case lineStreamErr:
		// The merge of several pods fails as a whole, under no pod.
		prefix := "stream error: "
		if ln.pod != "" {
			prefix = "[" + ln.pod + "] " + prefix
		}
		return styles.Warn.Render(prefix) + paint(ln.text, &styles.Warn, spans, current)
	case lineStreamDone:
		return styles.Hint.Render("[" + ln.pod + "] stream closed")
	}
	body := paint(ln.text, nil, spans, current)
	if !multi {
		return body
	}
	style := lipgloss.NewStyle().Foreground(styles.PodColor(colour))
	if full {
		return style.Bold(true).Render("["+ln.pod+"]") + " " + body
	}
	return style.Render(compactPrefix) + " " + body
}

// paint renders text in base (plain when nil) with the spans highlighted.
func paint(text string, base *lipgloss.Style, spans []span, current int) string {
	if len(spans) == 0 {
		return render(base, text)
	}
	var b strings.Builder
	at := 0
	for i, s := range spans {
		b.WriteString(render(base, text[at:s.start]))
		hl := styles.Highlight
		if i == current {
			hl = hl.Underline(true)
		}
		b.WriteString(hl.Render(text[s.start:s.end]))
		at = s.end
	}
	b.WriteString(render(base, text[at:]))
	return b.String()
}

func render(style *lipgloss.Style, s string) string {
	if style == nil || s == "" {
		return s
	}
	return style.Render(s)
}

// podPalette is ordered so neighbouring entries sit as far apart on the
// colour wheel as possible. Pods are coloured by position in the session's
// pod list, so the pods you actually compare side by side never share a hue
// family (the old FNV hash could hand out e.g. yellow and orange together).
// The entries are IDs; formatLine resolves them for the active theme.
var podPalette = styles.PodPalette()

// assignPodColors deals palette colours to pods round-robin by list position.
func assignPodColors(pods []string) map[string]styles.PodColorID {
	out := make(map[string]styles.PodColorID, len(pods))
	for i, p := range pods {
		out[p] = podPalette[i%len(podPalette)]
	}
	return out
}
