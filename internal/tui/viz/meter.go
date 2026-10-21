package viz

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Meter bands, in whole percent as the meter prints them.
const (
	WarnPercent = 70
	HotPercent  = 85
)

// HotGlyph follows the value of a meter at HotPercent or more. It is the
// waiting glyph (styles.GlyphWarn, which viz cannot import; a styles test
// keeps the two equal): a hot meter is a warning, never a failure, so ▲
// stays reserved for failing objects.
const HotGlyph = "△"

// Level is the band a meter value falls in.
type Level int

// Meter levels.
const (
	// LevelOK is below WarnPercent.
	LevelOK Level = iota
	// LevelWarn is WarnPercent up to HotPercent.
	LevelWarn
	// LevelHot is HotPercent or more: the warn style plus HotGlyph.
	LevelHot
)

// MeterLevel returns the band of percent rounded to a whole percent, the
// way the meter prints it, so 84.6 is hot because it reads "85%".
func MeterLevel(percent float64) Level {
	switch p := wholePercent(percent); {
	case p >= HotPercent:
		return LevelHot
	case p >= WarnPercent:
		return LevelWarn
	}
	return LevelOK
}

const (
	meterWide   = 12
	meterNarrow = 6
	// meterValue is "%3d%%" plus the hot mark column.
	meterValue = 5
)

// partials are the eighth blocks that end a fill, by eighths left over.
var partials = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// Meter draws percent (0 to 100) as a bar of eighth blocks on a · track,
// then its value, exactly width columns: 12 cells while width allows, then
// 6, then the value alone. The value is always printed, with HotGlyph from
// HotPercent up. Nothing paints with the err style; only the number and
// the glyph carry the warn attributes, the bar fill just its colour. progress (0 to 1) scales only the drawn fill, for the
// spring; the value and its level always show percent.
func Meter(th Theme, width int, percent, progress float64) string {
	if width <= 0 {
		return ""
	}
	p, lvl := wholePercent(percent), MeterLevel(percent)
	cells := meterCells(width)
	if cells == 0 {
		return pad(meterNumber(th, lvl, p, width), width)
	}
	frac := within(percent, 100) / 100 * within(progress, 1)
	return pad(meterBar(th, lvl, frac, cells)+" "+meterNumber(th, lvl, p, meterValue), width)
}

func meterCells(width int) int {
	switch {
	case width >= meterWide+1+meterValue:
		return meterWide
	case width >= meterNarrow+1+meterValue:
		return meterNarrow
	}
	return 0
}

func meterBar(th Theme, lvl Level, frac float64, cells int) string {
	eighths := int(math.Round(frac * float64(cells*8)))
	full, part := eighths/8, eighths%8
	used := full
	if part > 0 {
		used++
	}
	fill := strings.Repeat("█", full) + partials[part]
	return paint(colourOnly(lvl.style(th)), fill) + paint(th.Muted, strings.Repeat("·", cells-used))
}

// meterNumber is the value in the widest form that fits width: " 87%▲",
// then "87%▲", "87▲" and "87", and an ellipsis when not even that fits.
// The widest form keeps the mark column blank on a meter that is not hot,
// so values line up in a column.
func meterNumber(th Theme, lvl Level, p, width int) string {
	mark, gap := HotGlyph, ""
	if lvl != LevelHot {
		mark, gap = "", " "
	}
	n := strconv.Itoa(p)
	forms := []struct{ num, mark, gap string }{
		{num: fmt.Sprintf("%3d%%", p), mark: mark, gap: gap},
		{num: n + "%", mark: mark},
		{num: n, mark: mark},
		{num: n},
	}
	for _, f := range forms {
		if ansi.StringWidth(f.num+f.mark+f.gap) <= width {
			lead := strings.TrimRight(f.num, "0123456789%")
			return lead + lvl.paintValue(th, f.num[len(lead):]) + paint(th.Warn, f.mark) + f.gap
		}
	}
	return ellipsis
}

func (l Level) style(th Theme) lipgloss.Style {
	switch l {
	case LevelHot, LevelWarn:
		return th.Warn
	}
	return th.OK
}

// colourOnly drops the bold and underline of st, so a bar keeps its colour
// while the attributes that mark state in mono stay on the number.
func colourOnly(st lipgloss.Style) lipgloss.Style {
	return st.Bold(false).Underline(false)
}

// paintValue leaves an ok value plain: only the bands worth a look stand
// out, in mono through bold and underline.
func (l Level) paintValue(th Theme, s string) string {
	if l == LevelOK {
		return s
	}
	return paint(l.style(th), s)
}

// wholePercent rounds percent to a whole number in 0..100.
func wholePercent(percent float64) int {
	return int(math.Round(within(percent, 100)))
}
