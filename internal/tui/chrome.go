package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/cellbuf"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
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
// screen buffer.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
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
// body, its content cut to fit inside the card.
func (m Model) renderPopup(width, height int) string {
	inner := width - styles.PopupBox.GetHorizontalFrameSize()
	box := styles.PopupBox.Render(m.overlay.View(inner))
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}

func (m Model) renderHeader() string {
	parts := []string{styles.Title.Render("k4s")}
	if m.client != nil {
		parts = append(parts, styles.Hint.Render("ctx: "+m.client.Context))
	}
	ns := m.namespace
	if ns == "" {
		ns = "ALL"
	}
	parts = append(parts,
		styles.Hint.Render("ns: "+ns),
		styles.Hint.Render("view: "+m.current.Title()),
	)

	w := max(m.width-2, 0)
	return fit(styles.Header, w, strings.Join(parts, "  ·  "))
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
		return statusStyle(msg.Level).Render(ansi.Truncate(msg.Text, width, "…"))
	}
	return styles.Hint.Render(ansi.Truncate(strings.Join(m.footerBindings(), "  ·  "), width, "…"))
}

// footerCommandRow is the active command as the user would paste it, with
// ApproxMark in front when k4s does not do exactly what it says.
func (m Model) footerCommandRow(width int) string {
	shown := m.current.Command().Display()
	if shown == "" {
		return ""
	}
	return styles.KubectlHint.Render(ansi.Truncate(shown, width, "…"))
}

func (m Model) footerBindings() []string {
	quitLabel := "q quit"
	if m.current.Title() != viewDashboard {
		quitLabel = "q home"
	}
	bindings := []string{quitLabel, "^c quit", "esc back", ": command", "? help"}
	for _, b := range m.current.Help() {
		h := b.Help()
		if h.Key != "" && h.Desc != "" {
			bindings = append(bindings, h.Key+" "+h.Desc)
		}
	}
	return bindings
}
