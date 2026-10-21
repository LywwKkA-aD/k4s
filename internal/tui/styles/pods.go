package styles

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// podColors tell replicas apart in multi-pod logs. Neighbours sit far apart
// on the colour wheel, because pods are dealt colours by list position and
// the pods compared side by side should never share a hue family. Light
// variants are darker shades of the same hue.
var podColors = []token{
	{Light: "#6639BA", Dark: "#A78BFA"}, // purple
	{Light: "#1A7F37", Dark: "#04B575"}, // green
	{Light: "#9A4A00", Dark: "#F2A65A"}, // orange
	{Light: "#0B62A8", Dark: "#56B4D4"}, // blue
	{Light: "#B0306A", Dark: "#FF6B9D"}, // pink
	{Light: "#4D7A1F", Dark: "#A3BE8C"}, // lime
	{Light: "#7A6000", Dark: "#E5C07B"}, // yellow
	{Light: "#C0262D", Dark: "#F07178"}, // red
	{Light: "#00727A", Dark: "#3FC1C9"}, // cyan, also accent2
	{Light: "#8E3BA6", Dark: "#C678DD"}, // magenta
}

// accent2PodColor is the podColors entry the accent2 token reuses.
const accent2PodColor = 8

// PodColorID names one pod colour. Render it through PodColor so light and
// mono themes get their own variant.
type PodColorID string

// PodPalette returns one ID per pod colour, in the order they should be
// dealt. An ID is the colour's dark-theme hex.
func PodPalette() []PodColorID {
	ids := make([]PodColorID, len(podColors))
	for i, c := range podColors {
		ids[i] = PodColorID(c.Dark)
	}
	return ids
}

// PodColor returns the colour for a PodPalette ID under the active theme:
// its light or dark variant, or no colour at all in mono. Any other ID is
// read as a colour and returned unchanged outside mono.
func PodColor(id PodColorID) color.Color {
	if active.mono {
		return lipgloss.NoColor{}
	}
	for _, c := range podColors {
		if PodColorID(c.Dark) == id {
			return c.on(active.dark)
		}
	}
	return lipgloss.Color(string(id))
}
