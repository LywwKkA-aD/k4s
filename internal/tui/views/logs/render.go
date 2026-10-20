package logs

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

// toggleWrap flips wrap mode and resets the horizontal pan because the two
// are mutually exclusive: in wrap mode there is no "off to the right" to
// scroll to. The next syncViewport sees a clean xOffset=0.
func (m *Model) toggleWrap() {
	m.wrap = !m.wrap
	if m.wrap {
		m.xOffset = 0
	}
}

// hscroll moves the visible window by direction × hScrollStep columns.
// Clamped at 0 on the left; the right is loosely bounded by the widest
// rendered line so the user can scroll the entire content into view
// without overshooting too far into empty space.
func (m *Model) hscroll(direction int) {
	if m.wrap {
		return
	}
	step := direction * hScrollStep
	m.xOffset += step
	if m.xOffset < 0 {
		m.xOffset = 0
	}
	if maxOff := m.maxXOffset(); m.xOffset > maxOff {
		m.xOffset = maxOff
	}
}

// maxXOffset is the rightmost xOffset that still keeps something on screen.
// We use the widest rendered line minus a fudge so the user always sees a
// few columns of context. Viewport width is treated as a soft floor.
func (m Model) maxXOffset() int {
	if m.wrap {
		return 0
	}
	widest := 0
	for _, line := range m.rendered {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	visible := m.viewport.Width
	if visible <= 0 {
		visible = 1
	}
	max := widest - visible/2
	if max < 0 {
		return 0
	}
	return max
}

// syncViewport pushes the cached rendered lines into the viewport,
// applying display transforms (wrap or hscroll, then optional search
// highlight). Replaces the old refreshContent and is cheap because it
// reuses the pre-formatted cache instead of re-running formatRawLine
// per line per event.
func (m *Model) syncViewport() {
	var content string
	switch {
	case m.wrap && m.viewport.Width > 0:
		wrapped := make([]string, len(m.rendered))
		for i, line := range m.rendered {
			wrapped[i] = wrapLine(line, m.viewport.Width)
		}
		content = strings.Join(wrapped, "\n")
	case m.xOffset > 0:
		shifted := make([]string, len(m.rendered))
		for i, line := range m.rendered {
			shifted[i] = applyXOffset(line, m.xOffset)
		}
		content = strings.Join(shifted, "\n")
	default:
		content = strings.Join(m.rendered, "\n")
	}
	if m.searchQuery != "" {
		content = highlightContent(content, m.searchQuery)
	}
	m.viewport.SetContent(content)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// applyXOffset drops the first n display columns of an ANSI-aware string.
// ansi.TruncateLeft preserves SGR continuity so colour codes survive the
// crop. n=0 is the identity.
func applyXOffset(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.TruncateLeft(s, n, "")
}

// wrapLine soft-wraps an ANSI-coloured string to width. Reuses the same
// charmbracelet/x/ansi helper as the describe view — consistent behaviour
// across the app.
func wrapLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Wrap(s, width, "")
}

// formatRawLine turns a raw entry into the string the viewport renders.
//
// Modes:
//   - !multi: no prefix (single-pod view; pod name is in the title).
//   - multi && !full: a coloured "▌" bar — minimal but tells replicas apart.
//   - multi && full: the canonical "[pod-name] line" prefix.
//
// Stream-error and stream-closed notices keep the pod name inline so they
// remain readable regardless of the prefix mode.
func formatRawLine(ln logLine, multi, full bool, colour lipgloss.Color) string {
	switch ln.kind {
	case lineStreamErr:
		return styles.Warn.Render(fmt.Sprintf("[%s] stream error: %s", ln.pod, ln.text))
	case lineStreamDone:
		return styles.Hint.Render(fmt.Sprintf("[%s] stream closed", ln.pod))
	}

	if !multi {
		return ln.text
	}

	style := lipgloss.NewStyle().Foreground(styles.PodColor(colour))
	if full {
		return style.Bold(true).Render("["+ln.pod+"]") + " " + ln.text
	}
	return style.Render(compactPrefix) + " " + ln.text
}

// podPalette is ordered so neighbouring entries sit as far apart on the
// colour wheel as possible. Pods are coloured by position in the session's
// pod list, so the pods you actually compare side by side never share a hue
// family (the old FNV hash could hand out e.g. yellow and orange together).
// The entries are IDs; formatRawLine resolves them for the active theme.
var podPalette = styles.PodPalette()

// assignPodColors deals palette colours to pods round-robin by list position.
func assignPodColors(pods []string) map[string]lipgloss.Color {
	out := make(map[string]lipgloss.Color, len(pods))
	for i, p := range pods {
		out[p] = podPalette[i%len(podPalette)]
	}
	return out
}

func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}

	body := m.viewport.View()
	if len(m.rawLines) == 0 {
		body = styles.Hint.Render(fmt.Sprintf("waiting for logs… (tail %d)", m.tail))
	}

	top := make([]string, 0, statusRowsOpen)
	if m.searchOpen {
		top = append(top, m.searchInput.View(), "")
	} else {
		top = append(top, m.statusLine())
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(top, body)...)
}

// statusLine renders the one-line status above the viewport: any combination
// of "[paused]", "[tags on]", "[wrap on]", "[→ N]" and "/query · M/N".
// Empty when nothing applies — the row stays present so the layout does
// not jiggle.
func (m Model) statusLine() string {
	bits := make([]string, 0, 4)
	if !m.autoFollow {
		bits = append(bits, styles.Warn.Render("[paused — press f to follow]"))
	}
	if m.wrap {
		bits = append(bits, styles.Hint.Render("[wrap on — press w to disable]"))
	} else if m.xOffset > 0 {
		bits = append(bits, styles.Hint.Render(fmt.Sprintf("[→ %d cols]", m.xOffset)))
	}
	if len(m.pods) > 1 && m.showPodNames {
		bits = append(bits, styles.Hint.Render("[tags on — press t to compact]"))
	}
	switch {
	case m.searchQuery != "" && len(m.matches) > 0:
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · %d/%d (n/N step · c cancel)", m.searchQuery, m.matchIdx+1, len(m.matches))))
	case m.searchQuery != "":
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · no match (c cancel)", m.searchQuery)))
	}
	return strings.Join(bits, " · ")
}
