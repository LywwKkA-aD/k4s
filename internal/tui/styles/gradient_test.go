package styles

import (
	"fmt"
	"image/color"
	"slices"
	"sync"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestBlendStopsRunFromAccentToAccent2(t *testing.T) {
	t.Parallel()
	for _, dark := range []bool{true, false} {
		p := colourPalette(dark)
		stops := blendStops(p)
		if len(stops) != GradientSteps {
			t.Fatalf("dark=%v: got %d stops, want %d", dark, len(stops), GradientSteps)
		}
		if got, want := hexOf(stops[0]), hexOf(p.accent); got != want {
			t.Errorf("dark=%v: first stop %s, want accent %s", dark, got, want)
		}
		if got, want := hexOf(stops[len(stops)-1]), hexOf(p.accent2); got != want {
			t.Errorf("dark=%v: last stop %s, want accent2 %s", dark, got, want)
		}
		for i, s := range stops {
			if _, _, _, a := s.RGBA(); a != 0xFFFF {
				t.Errorf("dark=%v: stop %d is not opaque (alpha %#x)", dark, i, a)
			}
			if i > 0 && hexOf(s) == hexOf(stops[i-1]) {
				t.Errorf("dark=%v: stops %d and %d are both %s", dark, i-1, i, hexOf(s))
			}
		}
	}
}

// Stops are rounded to 8 bits, so what the contrast tests measure is
// exactly what a truecolor terminal is sent.
func TestBlendStopsAreEightBit(t *testing.T) {
	t.Parallel()
	for _, s := range blendStops(colourPalette(true)) {
		r, g, b, _ := s.RGBA()
		for _, v := range []uint32{r, g, b} {
			if v>>8 != v&0xFF {
				t.Errorf("stop %v channel %#x is not an 8-bit value", s, v)
			}
		}
	}
}

// The middle stops come from a CIELAB blend, which a plain sRGB average
// would not reproduce.
func TestBlendStopsAreLabNotSRGB(t *testing.T) {
	t.Parallel()
	p := colourPalette(true)
	mid := blendStops(p)[GradientSteps/2]
	ar, ag, ab, _ := p.accent.RGBA()
	br, bg, bb, _ := p.accent2.RGBA()
	avg := fmt.Sprintf("#%02X%02X%02X", (ar>>8+br>>8)/2, (ag>>8+bg>>8)/2, (ab>>8+bb>>8)/2)
	if hexOf(mid) == avg {
		t.Errorf("middle stop %s is the sRGB average", hexOf(mid))
	}
}

func TestBlendStopsInMonoAreColourless(t *testing.T) {
	t.Parallel()
	stops := blendStops(monoPalette)
	if len(stops) != GradientSteps {
		t.Fatalf("got %d stops, want %d", len(stops), GradientSteps)
	}
	for i, s := range stops {
		if _, ok := s.(lipgloss.NoColor); !ok {
			t.Errorf("mono stop %d = %v, want NoColor", i, s)
		}
	}
}

func TestGradientStopsFollowThemeAndAreCopies(t *testing.T) {
	t.Cleanup(resetTheme)
	for _, p := range []palette{colourPalette(true), colourPalette(false), monoPalette} {
		use(p)
		got := GradientStops()
		if want := blendStops(p); !slices.Equal(got, want) {
			t.Errorf("mono=%v dark=%v: GradientStops = %v, want %v", p.mono, p.dark, got, want)
		}
		got[0] = lipgloss.Color("#123456")
		if again := GradientStops(); again[0] == got[0] {
			t.Errorf("mono=%v dark=%v: writing to the returned stops changed the next call", p.mono, p.dark)
		}
	}
}

// rampIndexes maps each cell of a ramp back to the stop it shows.
func rampIndexes(t *testing.T, r Ramp, stops []color.Color) []int {
	t.Helper()
	out := make([]int, r.Len())
	for i := range out {
		out[i] = slices.Index(stops, r.At(i))
		if out[i] < 0 {
			t.Fatalf("cell %d = %v is not one of the stops", i, r.At(i))
		}
	}
	return out
}

// Every cell takes one of the tested stops, so a width never paints a
// colour the contrast tests did not check.
func TestGradientSpreadsStopsOverWidth(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	stops := GradientStops()
	cases := []struct {
		width int
		want  []int
	}{
		{width: 1, want: []int{0}},
		{width: 2, want: []int{0, 8}},
		{width: 3, want: []int{0, 4, 8}},
		{width: 4, want: []int{0, 3, 5, 8}},
		{width: 9, want: []int{0, 1, 2, 3, 4, 5, 6, 7, 8}},
	}
	for _, c := range cases {
		got := rampIndexes(t, Gradient(c.width), stops)
		if !slices.Equal(got, c.want) {
			t.Errorf("Gradient(%d) stops = %v, want %v", c.width, got, c.want)
		}
	}
	wide := rampIndexes(t, Gradient(120), stops)
	if wide[0] != 0 || wide[len(wide)-1] != GradientSteps-1 {
		t.Errorf("Gradient(120) runs from stop %d to %d, want 0 to %d", wide[0], wide[len(wide)-1], GradientSteps-1)
	}
	if !slices.IsSorted(wide) {
		t.Errorf("Gradient(120) is not monotonic: %v", wide)
	}
	for i := range GradientSteps {
		if !slices.Contains(wide, i) {
			t.Errorf("Gradient(120) skips stop %d", i)
		}
	}
}

func TestGradientEmptyAndWrapping(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	for _, w := range []int{0, -3} {
		r := Gradient(w)
		if r.Len() != 0 {
			t.Errorf("Gradient(%d).Len() = %d, want 0", w, r.Len())
		}
		if _, ok := r.At(0).(lipgloss.NoColor); !ok {
			t.Errorf("Gradient(%d).At(0) = %v, want NoColor", w, r.At(0))
		}
	}
	r := Gradient(5)
	cases := []struct{ i, want int }{{-1, 4}, {5, 0}, {7, 2}, {-6, 4}, {-10, 0}}
	for _, c := range cases {
		if r.At(c.i) != r.At(c.want) {
			t.Errorf("At(%d) = %v, want At(%d) = %v", c.i, r.At(c.i), c.want, r.At(c.want))
		}
	}
	var zero Ramp
	if _, ok := zero.At(3).(lipgloss.NoColor); !ok {
		t.Errorf("zero Ramp At = %v, want NoColor", zero.At(3))
	}
}

func TestGradientCachedPerThemeAndWidth(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	a, b := Gradient(40), Gradient(40)
	if &a.colors[0] != &b.colors[0] {
		t.Error("the same theme and width built the ramp twice")
	}
	if c := Gradient(41); &c.colors[0] == &a.colors[0] {
		t.Error("another width reused the cached ramp")
	}

	use(colourPalette(false))
	light := Gradient(40)
	if &light.colors[0] == &a.colors[0] {
		t.Error("a theme change kept the old theme's ramp")
	}
	if got, want := hexOf(light.At(0)), tokenAccent.Light; got != want {
		t.Errorf("light ramp starts at %s, want %s", got, want)
	}

	use(monoPalette)
	for i := range Gradient(40).Len() {
		if _, ok := Gradient(40).At(i).(lipgloss.NoColor); !ok {
			t.Fatalf("mono ramp cell %d = %v, want NoColor", i, Gradient(40).At(i))
		}
	}
}

// Resizing walks through many widths; the cache must not keep them all.
func TestGradientCacheIsBounded(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	for w := 1; w <= 3*maxCachedRamps; w++ {
		_ = Gradient(w)
	}
	gradients.mu.Lock()
	n := len(gradients.ramps)
	gradients.mu.Unlock()
	if n > maxCachedRamps {
		t.Errorf("cache holds %d ramps, want at most %d", n, maxCachedRamps)
	}
}

func TestGradientIsSafeForConcurrentUse(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range 2 * maxCachedRamps {
				r := Gradient(w + g)
				_ = r.At(w)
				_ = GradientStops()
			}
		}()
	}
	wg.Wait()
}
