package styles

import (
	"image/color"
	"slices"
	"sync"

	"charm.land/lipgloss/v2"
)

// GradientSteps is how many colours the accent to accent2 gradient holds.
// contrast_test.go checks each of them, and no width paints any other.
const GradientSteps = 9

// maxCachedRamps bounds the ramp cache: a drag-resize walks through many
// widths, but only the current few are ever drawn again.
const maxCachedRamps = 32

// maxRampWidth caps a ramp: no terminal is wider, and an absurd reported
// width must not allocate a matching slice. Wider runs get 1024 cells.
const maxRampWidth = 1024

// Ramp is the gradient spread over a run of cells. It is read-only, so one
// cached Ramp serves every caller.
type Ramp struct {
	colors []color.Color
}

// Len is the number of cells the ramp covers.
func (r Ramp) Len() int { return len(r.colors) }

// At returns the colour of cell i. Indexes wrap around, so a sweep can
// shift the ramp by any offset; an empty ramp returns NoColor.
func (r Ramp) At(i int) color.Color {
	n := len(r.colors)
	if n == 0 {
		return lipgloss.NoColor{}
	}
	return r.colors[((i%n)+n)%n]
}

// gradientCache holds the active theme's stops and the ramps built from
// them. Views render on the Bubble Tea goroutine, but tests and any
// future background renderer may not, so it is locked.
type gradientCache struct {
	mu    sync.Mutex
	stops []color.Color
	ramps map[int]Ramp
}

var gradients gradientCache

// reset installs a new theme's stops and drops every ramp of the old one.
func (g *gradientCache) reset(stops []color.Color) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stops = stops
	g.ramps = nil
}

func (g *gradientCache) ramp(width int) Ramp {
	if width <= 0 {
		return Ramp{}
	}
	width = min(width, maxRampWidth)
	g.mu.Lock()
	defer g.mu.Unlock()
	if r, ok := g.ramps[width]; ok {
		return r
	}
	if g.ramps == nil || len(g.ramps) >= maxCachedRamps {
		g.ramps = make(map[int]Ramp, maxCachedRamps)
	}
	r := spread(g.stops, width)
	g.ramps[width] = r
	return r
}

// GradientStops returns the GradientSteps colours of the accent to accent2
// CIELAB blend under the active theme, first to last. In mono every stop is
// NoColor. The slice is the caller's own.
func GradientStops() []color.Color {
	gradients.mu.Lock()
	defer gradients.mu.Unlock()
	return slices.Clone(gradients.stops)
}

// Gradient returns the gradient fitted to width cells, from accent in the
// first cell to accent2 in the last, each cell taking one of the
// GradientStops. It is cached per theme and width. A width below one gives
// an empty Ramp.
func Gradient(width int) Ramp {
	return gradients.ramp(width)
}

// spread lays the GradientSteps stops over width cells, rounding each cell
// to the nearest stop so both ends land exactly on the first and last.
func spread(stops []color.Color, width int) Ramp {
	if width == 1 {
		return Ramp{colors: []color.Color{stops[0]}}
	}
	colors := make([]color.Color, width)
	last, span := len(stops)-1, width-1
	for i := range colors {
		colors[i] = stops[(2*i*last+span)/(2*span)]
	}
	return Ramp{colors: colors}
}

// blendStops builds the gradient stops for p: lipgloss's CIELAB Blend1D
// from accent to accent2, rounded to the 8-bit colours a truecolor
// terminal is sent so the contrast tests measure what is drawn.
func blendStops(p palette) []color.Color {
	if p.mono {
		return slices.Repeat([]color.Color{lipgloss.NoColor{}}, GradientSteps)
	}
	blended := lipgloss.Blend1D(GradientSteps, p.accent, p.accent2)
	stops := make([]color.Color, len(blended))
	for i, c := range blended {
		stops[i] = rgb8(c)
	}
	return stops
}

// rgb8 rounds c to the nearest 8-bit opaque colour. Truncating the 16-bit
// channels instead would shift a channel by one about half the time.
func rgb8(c color.Color) color.RGBA {
	r, g, b, _ := c.RGBA()
	return color.RGBA{R: to8(r), G: to8(g), B: to8(b), A: 0xFF}
}

func to8(v uint32) uint8 {
	return uint8((v*0xFF + 0x7FFF) / 0xFFFF) //nolint:gosec // v is at most 0xFFFF, so the result is at most 0xFF
}
