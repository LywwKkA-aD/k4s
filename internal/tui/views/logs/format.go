package logs

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
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
// Stream errors and notices keep the pod name inline so they remain
// readable regardless of the prefix mode. A line's text is its plain end,
// which is how painted lines reuse the prefix and how a match's column is
// found again.
func formatRawLine(ln logLine, multi, full bool, colour styles.PodColorID) string {
	switch ln.kind {
	case lineStreamErr:
		return styles.Warn.Render(errorPrefix(ln.pod)) + styles.Warn.Render(ln.text)
	case lineNotice:
		return styles.Hint.Render(noticePrefix(ln.pod)) + styles.Hint.Render(ln.text)
	case lineAlert:
		return styles.Warn.Render(noticePrefix(ln.pod)) + styles.Warn.Render(ln.text)
	}
	if !multi {
		return ln.text
	}
	style := lipgloss.NewStyle().Foreground(styles.PodColor(colour))
	if full {
		return style.Bold(true).Render(podTag(ln.pod)) + " " + ln.text
	}
	return style.Render(compactPrefix) + " " + ln.text
}

// errorPrefix heads a stream error. The merge of several pods fails as a
// whole, under no pod.
func errorPrefix(pod string) string {
	if pod == "" {
		return "stream error: "
	}
	return podTag(pod) + " stream error: "
}

// noticePrefix heads k4s's own line about a pod's stream.
func noticePrefix(pod string) string {
	return podTag(pod) + " "
}

// podTag is "[pod]" with the name made inert at draw time: pod names come
// from the cluster, and lines keep the raw one for their streams.
func podTag(pod string) string {
	return "[" + sanitize.Cell(pod) + "]"
}

// marks are the escape sequences that turn a style on and off.
type marks struct {
	on, off string
}

// marksOf takes the sequences s wraps a word in, so painting a match is
// two string writes instead of a render.
func marksOf(s lipgloss.Style) marks {
	const probe = "x"
	r := s.Render(probe)
	i := strings.Index(r, probe)
	if i < 0 {
		return marks{}
	}
	return marks{on: r[:i], off: r[i+len(probe):]}
}

func (k marks) wrap(b *strings.Builder, s string) {
	if s == "" {
		return
	}
	b.WriteString(k.on)
	b.WriteString(s)
	b.WriteString(k.off)
}

// painter paints search matches on the text of a line, never on its
// prefix, so a match can never land inside a colour code. One is built
// per layout, from the styles of the active theme.
type painter struct {
	match, current, warn, hint marks
}

func newPainter() painter {
	return painter{
		match:   marksOf(styles.Highlight),
		current: marksOf(styles.Highlight.Underline(true)),
		warn:    marksOf(styles.Warn),
		hint:    marksOf(styles.Hint),
	}
}

// line draws ln, cached as rendered, with the occurrences in spans painted
// as matches; current (an index into spans, or noCurrent) is the one n/N
// stands on.
func (p painter) line(ln logLine, rendered string, spans []span, current int) string {
	switch ln.kind {
	case lineLog:
		return strings.TrimSuffix(rendered, ln.text) + p.paint(ln.text, marks{}, spans, current)
	case lineStreamErr:
		return styles.Warn.Render(errorPrefix(ln.pod)) + p.paint(ln.text, p.warn, spans, current)
	case lineNotice:
		return styles.Hint.Render(noticePrefix(ln.pod)) + p.paint(ln.text, p.hint, spans, current)
	case lineAlert:
		return styles.Warn.Render(noticePrefix(ln.pod)) + p.paint(ln.text, p.warn, spans, current)
	}
	return rendered
}

// paint writes text in base with the spans highlighted.
func (p painter) paint(text string, base marks, spans []span, current int) string {
	var b strings.Builder
	b.Grow(len(text) + len(spans)*(len(p.match.on)+len(p.match.off)))
	at := 0
	for i, s := range spans {
		base.wrap(&b, text[at:s.start])
		hl := p.match
		if i == current {
			hl = p.current
		}
		hl.wrap(&b, text[s.start:s.end])
		at = s.end
	}
	base.wrap(&b, text[at:])
	return b.String()
}

// podPalette is ordered so neighbouring entries sit as far apart on the
// colour wheel as possible. Pods are coloured by position in the session's
// pod list, so the pods you actually compare side by side never share a hue
// family (the old FNV hash could hand out e.g. yellow and orange together).
// The entries are IDs; formatRawLine resolves them for the active theme.
var podPalette = styles.PodPalette()

// assignPodColors deals palette colours to pods round-robin by list position.
func assignPodColors(pods []string) map[string]styles.PodColorID {
	out := make(map[string]styles.PodColorID, len(pods))
	for i, p := range pods {
		out[p] = podPalette[i%len(podPalette)]
	}
	return out
}
