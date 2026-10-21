package styles

// highContrastTokens are the high-contrast preset (K4S_THEME=high-contrast):
// every text colour reaches WCAG AAA, 7:1, on the backgrounds
// contrast_test.go measures, Dracula and Gruvbox included, and so do the
// borders, which draw in muted and accent. Dark variants are pale shades of
// the default theme's hues, light variants deep ones, so a state keeps its
// colour family.
var highContrastTokens = tokenSet{
	muted:   token{Light: "#3D3D3D", Dark: "#C8C8C8"},
	accent:  token{Light: "#3E1F9E", Dark: "#D0C2FF"},
	accent2: hcPodColors[accent2PodColor],
	ok:      token{Light: "#0B5323", Dark: "#7EE2A8"},
	warn:    token{Light: "#5E3B00", Dark: "#FFD27A"},
	err:     token{Light: "#9B111B", Dark: "#FFADA8"},

	selectionFg: token{Light: "#FFFFFF", Dark: "#000000"},
	selectionBg: token{Light: "#3E1F9E", Dark: "#D0C2FF"},
	matchFg:     tokenMatchFg,
	matchBg:     tokenMatchBg,
	pods:        hcPodColors,
}

// hcPodColors are podColors at 7:1, hue for hue and in the same order.
var hcPodColors = []token{
	{Light: "#4A1D96", Dark: "#D0C2FF"}, // purple
	{Light: "#0B5323", Dark: "#7EE2A8"}, // green
	{Light: "#6B2E00", Dark: "#FFC48A"}, // orange
	{Light: "#0A3F75", Dark: "#9DD4FF"}, // blue
	{Light: "#7D1446", Dark: "#FFB3D1"}, // pink
	{Light: "#2F4F0D", Dark: "#C8E68C"}, // lime
	{Light: "#544200", Dark: "#F5DC82"}, // yellow
	{Light: "#8A1218", Dark: "#FFADA8"}, // red
	{Light: "#0B4A52", Dark: "#7FE7F0"}, // cyan, also accent2
	{Light: "#661A7A", Dark: "#EBB8FF"}, // magenta
}

// highContrastPalette is the high-contrast preset for a dark or a light
// background.
func highContrastPalette(dark bool) palette {
	return highContrastTokens.resolve(dark)
}
