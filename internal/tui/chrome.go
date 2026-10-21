package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/cellbuf"

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

// render composes header / body / footer. The body normally renders the
// active view, but multi-field prompts (tail lines, container picker) are
// drawn as centered popups that temporarily cover the view — the single-
// line ":" command bar stays in the footer because it is one line by
// design and a popup would feel heavyweight.
func (m Model) render() string {
	header := m.renderHeader()
	footer := m.renderFooter()

	bodyHeight := m.bodyHeight()
	bodyWidth := max(m.width, 0)

	content := m.current.View()
	if m.cmdMode == cmdBarTailPrompt || m.cmdMode == cmdBarContainerPrompt ||
		m.cmdMode == cmdBarForwardPrompt || m.cmdMode == cmdBarHelp ||
		m.cmdMode == cmdBarRestorePrompt {
		content = m.renderPromptPopup(bodyWidth, bodyHeight)
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

// renderPromptPopup renders the active prompt as a centred bordered card.
// Tail prompt is a textinput; container prompt is a navigable list.
func (m Model) renderPromptPopup(width, height int) string {
	var inner string
	switch m.cmdMode {
	case cmdBarTailPrompt:
		inner = lipgloss.JoinVertical(
			lipgloss.Left,
			styles.PopupTitle.Render("tail lines"),
			"",
			m.cmdBarView(),
			"",
			styles.Hint.Render("Enter run · Esc cancel · default 100"),
		)
	case cmdBarForwardPrompt:
		inner = m.renderForwardPromptInner()
	case cmdBarContainerPrompt:
		inner = m.renderContainerPickerInner()
	case cmdBarHelp:
		inner = m.renderHelpPopupInner()
	case cmdBarRestorePrompt:
		inner = m.renderRestorePromptInner()
	default:
		return ""
	}
	box := styles.PopupBox.Render(inner)
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

func (m Model) renderFooter() string {
	w := max(m.width-2, 0)

	// Only the single-line ":" command renders inline in the footer.
	// Multi-field prompts (tail / container) live in a centred popup
	// drawn in the body, so the footer stays informative.
	if m.cmdMode == cmdBarCommand {
		return fit(styles.Footer, w, m.cmdBarView())
	}

	var top string
	if m.cmdError != "" {
		top = styles.Warn.Render(m.cmdError)
	} else {
		top = styles.Hint.Render(strings.Join(m.footerBindings(), "  ·  "))
	}

	bottom := ""
	if eq := m.current.KubectlEquivalent(); eq != "" {
		bottom = styles.KubectlHint.Render("≈ " + eq)
	}

	if bottom == "" {
		return fit(styles.Footer, w, top)
	}
	return fit(styles.Footer, w, lipgloss.JoinVertical(lipgloss.Left, top, bottom))
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
