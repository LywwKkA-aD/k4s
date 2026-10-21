package logs

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// fed returns m after one drain tick delivering lines from pod "p".
func fed(t *testing.T, m Model, lines ...string) Model {
	t.Helper()
	events := make([]logEvent, len(lines))
	for i, l := range lines {
		events[i] = logEvent{Pod: "p", Line: l}
	}
	next, _ := m.Update(logEventsBatch{gen: m.sess.gen, events: events})
	out, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return out
}

func numbered(from, to int, format string) []string {
	out := make([]string, 0, to-from)
	for i := from; i < to; i++ {
		out = append(out, fmt.Sprintf(format, i))
	}
	return out
}

// sized is a one-pod view laid out 60 columns wide and 20 rows high.
func sized(t *testing.T) Model {
	t.Helper()
	m := New(nil, "ns", []string{"p"}, 100, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	return next.(Model)
}

// topRow is the first row the viewport shows.
func topRow(m Model) string {
	first, _, _ := strings.Cut(ansi.Strip(m.viewport.View()), "\n")
	return strings.TrimRight(first, " ")
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(tuitest.Press(k))
		m = next.(Model)
	}
	return m
}

func TestTrimKeepsAPausedViewOnItsLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		format string
		wrap   bool
	}{
		{name: "short lines", format: "line %04d"},
		{name: "wrapped lines", format: "line %04d " + strings.Repeat("pad ", 30), wrap: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := sized(t)
			if tc.wrap {
				m = press(t, m, "w")
			}
			m = fed(t, m, numbered(0, views.MaxLogLines, tc.format)...)
			m = press(t, m, "pgup", "pgup", "pgup")
			if m.autoFollow {
				t.Fatal("pgup did not pause")
			}
			before := topRow(m)

			// More than a buffer's worth in total, in several ticks.
			for from := views.MaxLogLines; from < views.MaxLogLines+300; from += 100 {
				m = fed(t, m, numbered(from, from+100, tc.format)...)
				if got := topRow(m); got != before {
					t.Fatalf("after line %d the top row moved: %q, was %q", from+100, got, before)
				}
			}
			if len(m.rawLines) != views.MaxLogLines {
				t.Fatalf("buffer holds %d lines, want %d", len(m.rawLines), views.MaxLogLines)
			}
		})
	}
}

func TestTrimDropsThePausedLineItself(t *testing.T) {
	t.Parallel()

	m := fed(t, sized(t), numbered(0, views.MaxLogLines, "line %04d")...)
	m = press(t, m, "up")
	m.viewport.GotoTop()
	m = fed(t, m, numbered(views.MaxLogLines, views.MaxLogLines+10, "line %04d")...)
	if got := topRow(m); got != "line 0010" {
		t.Fatalf("top row = %q, want the oldest line left", got)
	}
}

func TestTrimShiftsMatchesInOrder(t *testing.T) {
	t.Parallel()

	const format = "line %d tick"
	m := fed(t, sized(t), numbered(0, views.MaxLogLines, format)...)
	m = press(t, m, "/", "l", "i", "n", "e", "space", "4", "enter")
	// "line 4" is line 4, then 40..49, 400..499 and 4000..4999.
	current := func() string { return m.rawLines[m.matches[m.matchIdx].line].text }
	steps := []struct {
		feed  int // lines appended before the keys
		keys  []string
		want  string
		count string
	}{
		{want: "line 4 tick", count: "1/1111"},
		{keys: []string{"n", "n"}, want: "line 41 tick", count: "3/1111"},
		// Lines 0..39 go: line 4 with them, line 41 stays current.
		{feed: 40, want: "line 41 tick", count: "2/1110"},
		{keys: []string{"n"}, want: "line 42 tick", count: "3/1110"},
		{keys: []string{"N", "N", "N"}, want: "line 4999 tick", count: "1110/1110"},
		{keys: []string{"n"}, want: "line 40 tick", count: "1/1110"},
		// The current line itself goes: the first match left takes over.
		{feed: 5, want: "line 45 tick", count: "1/1105"},
	}
	next := views.MaxLogLines
	for i, s := range steps {
		if s.feed > 0 {
			m = fed(t, m, numbered(next, next+s.feed, format)...)
			next += s.feed
		}
		m = press(t, m, s.keys...)
		if got := current(); got != s.want {
			t.Fatalf("step %d: current match = %q, want %q", i, got, s.want)
		}
		if !strings.Contains(m.Title(), " "+s.count+" ") {
			t.Fatalf("step %d: title %q, want counter %s", i, m.Title(), s.count)
		}
	}
}

func TestMatchesFollowNewLines(t *testing.T) {
	t.Parallel()

	m := fed(t, sized(t), "alpha", "beta")
	m = press(t, m, "/", "a", "enter")
	if len(m.matches) != 3 {
		t.Fatalf("matches = %v", m.matches)
	}
	m = fed(t, m, "gamma", "delta")
	if got, want := len(m.matches), 6; got != want {
		t.Fatalf("matches after new lines = %d, want %d", got, want)
	}
	for i := 1; i < len(m.matches); i++ {
		a, b := m.matches[i-1], m.matches[i]
		if b.line < a.line || (b.line == a.line && b.start <= a.start) {
			t.Fatalf("matches out of order: %v", m.matches)
		}
	}
}
