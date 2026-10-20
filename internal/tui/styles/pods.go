package styles

import "github.com/charmbracelet/lipgloss"

// podColors tell replicas apart in multi-pod logs. Neighbours sit far apart
// on the colour wheel, because pods are dealt colours by list position and
// the pods compared side by side should never share a hue family. Light
// variants are darker shades of the same hue.
var podColors = []lipgloss.AdaptiveColor{
	{Light: "#6639BA", Dark: "#A78BFA"}, // purple
	{Light: "#1A7F37", Dark: "#04B575"}, // green
	{Light: "#9A4A00", Dark: "#F2A65A"}, // orange
	{Light: "#0B62A8", Dark: "#56B4D4"}, // blue
	{Light: "#B0306A", Dark: "#FF6B9D"}, // pink
	{Light: "#4D7A1F", Dark: "#A3BE8C"}, // lime
	{Light: "#7A6000", Dark: "#E5C07B"}, // yellow
	{Light: "#C0262D", Dark: "#F07178"}, // red
	{Light: "#00727A", Dark: "#3FC1C9"}, // cyan
	{Light: "#8E3BA6", Dark: "#C678DD"}, // magenta
}

// PodPalette returns one ID per pod colour, in the order they should be
// dealt. An ID is the colour's dark-theme hex; render it through PodColor
// so light and mono themes get their own variant.
func PodPalette() []lipgloss.Color {
	ids := make([]lipgloss.Color, len(podColors))
	for i, c := range podColors {
		ids[i] = lipgloss.Color(c.Dark)
	}
	return ids
}

// PodColor returns the colour for a PodPalette ID under the active theme:
// its adaptive pair, or no colour at all in mono. Any other colour is
// returned unchanged outside mono.
func PodColor(id lipgloss.Color) lipgloss.TerminalColor {
	if active.mono {
		return lipgloss.NoColor{}
	}
	for _, c := range podColors {
		if lipgloss.Color(c.Dark) == id {
			return c
		}
	}
	return id
}
