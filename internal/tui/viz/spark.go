package viz

import (
	"math"
	"strings"
)

// blocks are the sparkline bars, lowest first.
var blocks = [8]string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// Braille dot bits per cell column, bottom dot first: dots 7, 3, 2, 1 on
// the left and 8, 6, 5, 4 on the right of the U+2800 block.
var (
	leftDots  = [4]rune{0x40, 0x04, 0x02, 0x01}
	rightDots = [4]rune{0x80, 0x20, 0x10, 0x08}
)

// Spark draws the newest width samples of series as block bars in Accent2,
// oldest on the left: ▁ for 0 up to █ for maxV. A NaN sample (no data, or
// time spent away from the dashboard) is a blank, and so is every column
// older than the series. maxV <= 0 scales to the largest sample shown.
func Spark(th Theme, width int, series []float64, maxV float64) string {
	if width <= 0 {
		return ""
	}
	shown := newest(series, width)
	top := scaleMax(shown, maxV)
	var b strings.Builder
	b.WriteString(strings.Repeat(" ", width-len(shown)))
	for _, v := range shown {
		if math.IsNaN(v) {
			b.WriteByte(' ')
			continue
		}
		b.WriteString(blocks[steps(v, top, len(blocks)-1)])
	}
	return paint(th.Accent2, b.String())
}

// Braille draws series as an area chart height rows tall in Accent2. Each
// cell holds two samples, older on the left, and four dots a row, so a
// chart width cells wide shows the newest 2*width samples. A sample that is
// there shows at least its bottom dot, so 0 reads as a baseline; NaN
// samples and columns older than the series are blank. maxV <= 0 scales to
// the largest sample shown. Rows are joined with "\n".
func Braille(th Theme, width, height int, series []float64, maxV float64) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	shown := newest(series, 2*width)
	top := scaleMax(shown, maxV)
	dots := make([]int, 2*width)
	offset := len(dots) - len(shown)
	for i, v := range shown {
		if !math.IsNaN(v) {
			dots[offset+i] = max(1, steps(v, top, 4*height))
		}
	}
	rows := make([]string, height)
	for r := range rows {
		floor := 4 * (height - 1 - r)
		var b strings.Builder
		for c := range width {
			b.WriteString(brailleCell(dots[2*c]-floor, dots[2*c+1]-floor))
		}
		rows[r] = paint(th.Accent2, b.String())
	}
	return strings.Join(rows, "\n")
}

// brailleCell fills the bottom left and right dots of one cell; a cell
// with no dots is a space, so gaps stay blank in every font.
func brailleCell(left, right int) string {
	var bits rune
	for i := range 4 {
		if left > i {
			bits |= leftDots[i]
		}
		if right > i {
			bits |= rightDots[i]
		}
	}
	if bits == 0 {
		return " "
	}
	return string(0x2800 + bits)
}

// newest returns the last n samples of series.
func newest(series []float64, n int) []float64 {
	if len(series) > n {
		return series[len(series)-n:]
	}
	return series
}

// scaleMax is maxV when it is set, otherwise the largest finite sample, and
// 1 when there is none above 0 so an all-zero series still draws.
func scaleMax(series []float64, maxV float64) float64 {
	if maxV > 0 {
		return maxV
	}
	top := 0.0
	for _, v := range series {
		if !math.IsNaN(v) && !math.IsInf(v, 0) {
			top = math.Max(top, v)
		}
	}
	if top <= 0 {
		return 1
	}
	return top
}

// steps maps v in [0, top] to a whole number of steps in [0, n].
func steps(v, top float64, n int) int {
	return int(math.Round(within(v/top, 1) * float64(n)))
}
