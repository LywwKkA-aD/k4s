package viz_test

import (
	"errors"
	"image/color"
	"math"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

func mustHex(t *testing.T, s string) color.RGBA {
	t.Helper()
	c, err := viz.ParseHex(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestContrastRatioMatchesWCAGReferences(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		a, b string
		want float64
	}{
		{name: "black on white", a: "#000000", b: "#FFFFFF", want: 21},
		{name: "symmetric", a: "#FFFFFF", b: "#000000", want: 21},
		{name: "same colour", a: "#7D56F4", b: "#7D56F4", want: 1},
		{name: "grey 777 on white", a: "#777777", b: "#FFFFFF", want: 4.48},
		{name: "selected row", a: "#FFFFFF", b: "#7D56F4", want: 4.66},
		{name: "accent2 on dracula, the spec minimum", a: "#3FC1C9", b: "#282A36", want: 6.57},
		{name: "lowercase hex", a: "#ffffff", b: "#7d56f4", want: 4.66},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := viz.ContrastRatio(mustHex(t, c.a), mustHex(t, c.b))
			if math.Abs(got-c.want) > 0.01 {
				t.Errorf("ContrastRatio(%s, %s) = %.3f, want %.2f", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestLuminanceEnds(t *testing.T) {
	t.Parallel()
	if got := viz.Luminance(color.Black); got != 0 {
		t.Errorf("Luminance(black) = %v, want 0", got)
	}
	if got := viz.Luminance(color.White); math.Abs(got-1) > 1e-9 {
		t.Errorf("Luminance(white) = %v, want 1", got)
	}
	// The low segment of the sRGB curve: 10/255 is below 0.04045.
	if got, want := viz.Luminance(color.RGBA{R: 10, G: 10, B: 10, A: 255}), 10.0/255/12.92; math.Abs(got-want) > 1e-9 {
		t.Errorf("Luminance(#0A0A0A) = %v, want %v", got, want)
	}
}

func TestParseHex(t *testing.T) {
	t.Parallel()
	if got := mustHex(t, "#00727A"); got != (color.RGBA{R: 0x00, G: 0x72, B: 0x7A, A: 0xFF}) {
		t.Errorf("ParseHex(#00727A) = %v", got)
	}
	for _, s := range []string{"", "8", "240", "#FFF", "#GGGGGG", "7D56F4", "#7D56F4F", "#+12345"} {
		if _, err := viz.ParseHex(s); !errors.Is(err, viz.ErrNotHex) {
			t.Errorf("ParseHex(%q) error = %v, want ErrNotHex", s, err)
		}
	}
}
