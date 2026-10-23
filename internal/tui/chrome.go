package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// bodyHeight returns the number of lines available between header and footer,
// computed from the *current* chrome so we match whatever lipgloss actually
// rendered.
func (m Model) bodyHeight() int {
	hh := lipgloss.Height(m.renderHeader())
	fh := lipgloss.Height(m.renderFooter())
	h := m.height - hh - fh
	if h < 1 {
		return 1
	}
	return h
}

// View is the full-screen frame: the composed screen on the alternate
// screen buffer, blank while the first frame waits for the terminal's
// background.
func (m Model) View() tea.View {
	content := ""
	if !m.holdFrame {
		content = m.render()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = m.windowTitle()
	// The dashboard polls slower while the terminal is in the background.
	v.ReportFocus = true
	return v
}

// render composes header / body / footer. A popup overlay covers the
// body while it is open; the command bar is one line by design and sits in
// the footer instead.
func (m Model) render() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	bodyHeight := m.bodyHeight()
	bodyWidth := max(m.width, 0)

	content := m.current.View()
	if m.overlay != nil && overlay.PlacementOf(m.overlay) == overlay.Popup {
		content = m.renderPopup(bodyWidth, bodyHeight)
	}

	body := fit(lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight), bodyWidth, content)

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

// fit renders s with st at width w. Lines wider than the space inside the
// padding are wrapped with x/cellbuf first, as the chrome has always been:
// lipgloss's own wrap breaks styled lines elsewhere, which moves the
// selected row of a table that is wider than the terminal.
func fit(st lipgloss.Style, w int, s string) string {
	if wrapAt := w - st.GetHorizontalPadding(); wrapAt > 0 {
		s = cellbuf.Wrap(s, wrapAt, "")
	}
	return st.Width(w).Render(s)
}

// renderPopup draws the open overlay as a bordered card centred in the
// body, its content cut to fit inside the card. A card taller than the
// body keeps its border: the content loses rows at the bottom instead,
// the last row left saying so.
func (m Model) renderPopup(width, height int) string {
	inner := width - styles.PopupBox.GetHorizontalFrameSize()
	room := max(height-styles.PopupBox.GetVerticalFrameSize(), 1)
	box := styles.PopupBox.Render(cutRows(m.overlay.View(inner), room))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

// cutRows is s cut to its first n rows, the last of them "…" when any
// were cut; a single row is kept as it is.
func cutRows(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	kept := slices.Clone(lines[:n])
	if n > 1 {
		kept[n-1] = "…"
	}
	return strings.Join(kept, "\n")
}

// renderHeader is one line over a rule. The line is fitted to the width
// (see layoutHeader), so the body height never depends on what it says.
func (m Model) renderHeader() string {
	w := max(m.width-2, 0)
	inner := max(w-styles.Header.GetHorizontalPadding(), 0)
	return fit(styles.Header, w, layoutHeader(m.headerParts(), inner))
}

// renderFooter is always two rows under the rule: the command bar, the
// status message or the key hints first, the kubectl command second. Each
// row is cut to the width with an ellipsis, so the body height never
// depends on what the footer says.
func (m Model) renderFooter() string {
	w := max(m.width-2, 0)
	cw := max(w-styles.Footer.GetHorizontalFrameSize(), 1)
	rows := []string{m.footerTopRow(cw), m.footerCommandRow(cw)}
	return styles.Footer.Width(w).Render(strings.Join(rows, "\n"))
}

func (m Model) footerTopRow(width int) string {
	if m.overlay != nil && overlay.PlacementOf(m.overlay) == overlay.Footer {
		return m.overlay.View(width)
	}
	if msg, ok := m.status.Current(m.now()); ok {
		return statusStyle(msg.Level).Render(ansi.Truncate(statusMark(msg.Level)+msg.Text, width, "…"))
	}
	return styles.Hint.Render(ansi.Truncate(strings.Join(m.footerBindings(), "  ·  "), width, "…"))
}

// statusMark leads a status message with the dashboard's glyph for its
// level, so the level never shows by colour alone: mono, NO_COLOR and a
// reader who cannot tell the colours apart still see it. Info is neutral
// progress and has none.
func statusMark(level status.Level) string {
	switch level {
	case status.OK:
		return styles.GlyphOK + " "
	case status.Warn:
		return styles.GlyphWarn + " "
	case status.Error, status.Alert:
		return styles.GlyphErr + " "
	}
	return ""
}

// footerCommandRow is the active command as the user would paste it:
// with the --context and --kubeconfig it needs to reach this cluster, and
// ApproxMark in front when k4s does not do exactly what it says.
// Quoting keeps a namespace or context from the kubeconfig whole for the
// shell, not inert for the terminal, so the row is drawn sanitized while
// y copies the command as it is.
func (m Model) footerCommandRow(width int) string {
	shown := sanitize.Line(m.decorate(m.activeCommand()).Display())
	if shown == "" {
		return ""
	}
	return styles.KubectlHint.Render(ansi.Truncate(shown, width, "…"))
}

// footerBindings are the key hints: the root's keys, the view's, then o
// on a view with an object to open (views.Selection; elsewhere o just
// says there is none). o comes last because the row is cut to the width:
// ahead of the view's hints it would push them, the watch state among
// them, off a narrow terminal. The help lists o on every view.
func (m Model) footerBindings() []string {
	quitLabel := "q quit"
	if m.current.Title() != viewDashboard {
		quitLabel = "q home"
	}
	bindings := []string{quitLabel, "^c quit", "esc back", ": command", "? help", "y copy"}
	for _, b := range m.current.Help() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			bindings = append(bindings, h.Key+" "+h.Desc)
		}
	}
	if _, ok := m.current.(views.Selection); ok {
		bindings = append(bindings, m.keys.YAML.Help().Key+" yaml")
	}
	return bindings
}

// activeCommand is the command the footer's second row shows: the open
// overlay's when it previews one (the tail prompt, the container picker),
// else the one the status row is about (the kubectl debug command after a
// missing shell), else the active view's.
func (m Model) activeCommand() kubectl.Cmd {
	cmd, _ := m.commandOver(m.status.Command(m.now()))
	return cmd
}

// commandOver is activeCommand given the status row's command, if any;
// about reports that the status supplied it.
func (m Model) commandOver(statusCmd kubectl.Cmd, shown bool) (cmd kubectl.Cmd, about bool) {
	if cmd, ok := overlay.CommandOf(m.overlay); ok {
		return cmd, false
	}
	if shown {
		return statusCmd, true
	}
	return m.current.Command(), false
}
