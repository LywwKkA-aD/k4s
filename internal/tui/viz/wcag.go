package viz

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image/color"
	"math"
)

// ErrNotHex reports a colour that is not written #RRGGBB.
var ErrNotHex = errors.New("want #RRGGBB")

// ParseHex parses a #RRGGBB colour, the form every styles token uses.
// ANSI palette indexes are rejected: they resolve through the terminal's
// own scheme, so their contrast cannot be measured.
func ParseHex(s string) (color.RGBA, error) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, fmt.Errorf("parse colour %q: %w", s, ErrNotHex)
	}
	rgb, err := hex.DecodeString(s[1:])
	if err != nil {
		return color.RGBA{}, fmt.Errorf("parse colour %q: %w", s, ErrNotHex)
	}
	return color.RGBA{R: rgb[0], G: rgb[1], B: rgb[2], A: 0xFF}, nil
}

// Luminance returns the WCAG 2.x relative luminance of an opaque colour,
// from 0 for black to 1 for white.
func Luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return 0.2126*linear(r) + 0.7152*linear(g) + 0.0722*linear(b)
}

// ContrastRatio returns the WCAG 2.x contrast ratio of two opaque colours,
// from 1 to 21. Text needs 4.5 for AA.
func ContrastRatio(a, b color.Color) float64 {
	la, lb := Luminance(a), Luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

// linear undoes the sRGB transfer curve for one 16-bit channel.
func linear(v uint32) float64 {
	c := float64(v) / 0xFFFF
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
