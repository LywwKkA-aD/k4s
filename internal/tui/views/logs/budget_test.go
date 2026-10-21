package logs

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// longLines are n lines of nearly sanitize.MaxLine bytes, each tagged with
// its number so the newest can be told apart.
func longLines(from, n int) []string {
	pad := strings.Repeat("x", sanitize.MaxLine-16)
	out := make([]string, 0, n)
	for i := from; i < from+n; i++ {
		out = append(out, "line "+strconv.Itoa(i)+" "+pad)
	}
	return out
}

func backlogBytes(m Model) int {
	n := 0
	for _, ln := range m.rawLines {
		n += len(ln.text)
	}
	return n
}

func TestBacklogKeepsToItsByteBudget(t *testing.T) {
	t.Parallel()

	m := sized(t)
	for from := 0; from < 600; from += 100 {
		m = fed(t, m, longLines(from, 100)...)
	}
	if got := backlogBytes(m); got > maxBacklogBytes {
		t.Fatalf("backlog holds %d bytes, budget %d", got, maxBacklogBytes)
	}
	if len(m.rawLines) >= views.MaxLogLines || len(m.rawLines) < maxBacklogBytes/sanitize.MaxLine {
		t.Fatalf("backlog holds %d lines of %d bytes", len(m.rawLines), sanitize.MaxLine)
	}
	if last := m.rawLines[len(m.rawLines)-1].text; !strings.HasPrefix(last, "line 599 ") {
		t.Fatalf("newest line kept = %.20q", last)
	}
	if m.bytes != backlogBytes(m) {
		t.Fatalf("byte count %d, lines hold %d", m.bytes, backlogBytes(m))
	}
}

func TestShortLinesStillFillTheLineBudget(t *testing.T) {
	t.Parallel()

	m := fed(t, sized(t), numbered(0, views.MaxLogLines+10, "line %d")...)
	if len(m.rawLines) != views.MaxLogLines {
		t.Fatalf("backlog holds %d lines, want %d", len(m.rawLines), views.MaxLogLines)
	}
	m.clearLogs()
	if m.bytes != 0 {
		t.Fatalf("cleared backlog counts %d bytes", m.bytes)
	}
}

// A batch lays out its own lines only: the rows of every earlier line are
// taken from the cache, whatever their length.
func TestBatchLaysOutOnlyItsNewLines(t *testing.T) {
	t.Parallel()

	for _, wrap := range []bool{false, true} {
		m := sized(t)
		if wrap {
			m = press(t, m, "w")
		}
		m = fed(t, m, longLines(0, 20)...)
		before := m.cells
		m = fed(t, m, "one more")
		if len(m.cells) != len(before)+1 {
			t.Fatalf("wrap %v: %d cells after one line, %d before", wrap, len(m.cells), len(before))
		}
		for i := range before {
			if &m.cells[i].rows[0] != &before[i].rows[0] {
				t.Fatalf("wrap %v: line %d was laid out again", wrap, i)
			}
		}
	}
}

// With wrap off the viewport gets each line cut to the window, so it never
// measures the whole of a long line.
func TestClippedRowsAreCutToTheWindow(t *testing.T) {
	t.Parallel()

	m := fed(t, sized(t), longLines(0, 3)...)
	m = press(t, m, "right", "right")
	for i, row := range strings.Split(m.viewport.GetContent(), "\n") {
		if w := ansi.StringWidth(row); w > m.viewport.Width() {
			t.Fatalf("row %d is %d columns wide, window %d", i, w, m.viewport.Width())
		}
	}
	if got := topRow(m); !strings.HasPrefix(got, strings.Repeat("x", 10)) {
		t.Fatalf("panned top row = %.30q", got)
	}
}

// A new search, n/N and a cancel repaint the lines they touch; the cache
// never shows a stale highlight.
func TestPaintFollowsTheSearch(t *testing.T) {
	t.Parallel()

	p := newPainter()
	m := fed(t, sized(t), "alpha", "beta", "alpha")
	m = press(t, m, "/", "a", "l", "enter")
	if got := strings.Count(m.viewport.GetContent(), p.current.on); got != 1 {
		t.Fatalf("%d current highlights after the search", got)
	}
	m = press(t, m, "n")
	rows := strings.Split(m.viewport.GetContent(), "\n")
	if strings.Contains(rows[0], p.current.on) || !strings.Contains(rows[2], p.current.on) {
		t.Fatalf("n left the current highlight behind:\n%q", rows)
	}
	m = press(t, m, "c")
	if strings.Contains(m.viewport.GetContent(), p.match.on) {
		t.Fatalf("a cancelled search left highlights:\n%q", m.viewport.GetContent())
	}
}
