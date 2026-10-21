package pager_test

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/pager"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// text is n numbered lines, every tenth of them naming the needle.
func text(n int) string {
	lines := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		line := fmt.Sprintf("line %02d", i)
		if i%10 == 0 {
			line += " Needle"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func feed(t *testing.T, m pager.Model, msgs ...tea.Msg) pager.Model {
	t.Helper()
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func screen(m pager.Model) []string {
	return strings.Split(tuitest.TrimLines(tuitest.Strip(m.View())), "\n")
}

func sized(content string, w, h int) pager.Model {
	return pager.New().SetSize(w, h).SetContent(content)
}

func TestPositionFollowsTheScroll(t *testing.T) {
	t.Parallel()

	if got := sized("short", 40, 10).Position(); got != "" {
		t.Errorf("content that fits: Position = %q, want none", got)
	}
	m := sized(text(50), 40, 10)
	if got := m.Position(); got != "top · ↓ more" {
		t.Errorf("at the top: Position = %q", got)
	}
	m = feed(t, m, tuitest.Press("down"))
	if got := m.Position(); !strings.HasSuffix(got, "%") {
		t.Errorf("in the middle: Position = %q", got)
	}
	m = feed(t, m, tuitest.Press("G"))
	if got := m.Position(); got != "end" {
		t.Errorf("after G: Position = %q", got)
	}
	if last := screen(m)[9]; last != "line 50 Needle" {
		t.Errorf("after G the last line is %q", last)
	}
	m = feed(t, m, tuitest.Press("g"))
	if first := screen(m)[0]; first != "line 01" {
		t.Errorf("after g the first line is %q", first)
	}
}

func TestContentWrapsToTheWidth(t *testing.T) {
	t.Parallel()

	m := sized("aaaa bbbb cccc dddd", 10, 5)
	got := screen(m)
	if got[0] != "aaaa bbbb" || got[1] != "cccc dddd" {
		t.Errorf("wrapped to 10: %q", got)
	}
	m = m.SetSize(30, 5)
	if got := screen(m)[0]; got != "aaaa bbbb cccc dddd" {
		t.Errorf("rewrapped to 30: %q", got)
	}
}

func TestSearchFindsAndStepsThroughMatches(t *testing.T) {
	t.Parallel()

	m := sized(text(50), 40, 10)
	m = feed(t, m, tuitest.Press("/"))
	if !m.CapturesKeys() {
		t.Fatal("the open prompt does not take the keys")
	}
	m = feed(t, m, tuitest.Type("needle"), tuitest.Enter())
	if m.CapturesKeys() || !m.Searching() {
		t.Fatalf("after Enter: captures=%v searching=%v", m.CapturesKeys(), m.Searching())
	}
	lines := screen(m)
	if bar := lines[len(lines)-1]; !strings.Contains(bar, "/ needle") || !strings.Contains(bar, "1/5") {
		t.Errorf("search bar = %q, want the query and 1/5", bar)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "line 10 Needle") {
		t.Errorf("the first match is not on screen:\n%s", strings.Join(lines, "\n"))
	}
	m = feed(t, m, tuitest.Press("n"), tuitest.Press("n"))
	if bar := last(screen(m)); !strings.Contains(bar, "3/5") || !strings.Contains(strings.Join(screen(m), "\n"), "line 30 Needle") {
		t.Errorf("after n n: %q", screen(m))
	}
	m = feed(t, m, tuitest.Press("N"), tuitest.Press("N"), tuitest.Press("N"))
	if bar := last(screen(m)); !strings.Contains(bar, "5/5") {
		t.Errorf("N wraps from the first match to the last: %q", bar)
	}
}

func TestSearchHighlightsMatches(t *testing.T) {
	t.Parallel()

	m := feed(t, sized("alpha\nbeta alpha", 40, 10), tuitest.Press("/"), tuitest.Type("ALPHA"), tuitest.Enter())
	raw := m.View()
	if tuitest.Strip(raw) == raw {
		t.Fatalf("matches are not highlighted:\n%q", raw)
	}
	if got := screen(m); got[0] != "alpha" || got[1] != "beta alpha" {
		t.Errorf("highlighting changed the text: %q", got)
	}
}

func TestSearchWithoutMatch(t *testing.T) {
	t.Parallel()

	m := feed(t, sized(text(5), 40, 10), tuitest.Press("/"), tuitest.Type("zzz"), tuitest.Enter())
	if bar := last(screen(m)); !strings.Contains(bar, "no match") {
		t.Errorf("search bar = %q, want no match", bar)
	}
	before := screen(m)
	m = feed(t, m, tuitest.Press("n"))
	if strings.Join(screen(m), "\n") != strings.Join(before, "\n") {
		t.Error("n without matches moved the view")
	}
}

// TestStepWithoutSearchSaysSo: n and N before any search answer in the
// status row instead of doing nothing.
func TestStepWithoutSearchSaysSo(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"n", "N"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			m := sized(text(50), 40, 10)
			next, cmd := m.Update(tuitest.Press(k))
			got, ok := tuitest.Find[views.StatusMsg](tuitest.Collect(t, cmd))
			want := views.StatusMsg{Level: status.Info, Text: "no search: press / first"}
			if !ok || got != want {
				t.Errorf("%s without a search said %+v, want %+v", k, got, want)
			}
			if strings.Join(screen(next), "\n") != strings.Join(screen(m), "\n") {
				t.Errorf("%s without a search moved the view", k)
			}
		})
	}
}

func TestSearchPromptEscKeepsTheCommittedQuery(t *testing.T) {
	t.Parallel()

	m := feed(t, sized(text(50), 40, 10), tuitest.Press("/"), tuitest.Type("needle"), tuitest.Enter())
	m = feed(t, m, tuitest.Press("/"), tuitest.Press("x"), tuitest.Esc())
	if m.CapturesKeys() || !m.Searching() {
		t.Fatalf("Esc in the prompt: captures=%v searching=%v", m.CapturesKeys(), m.Searching())
	}
	if bar := last(screen(m)); !strings.Contains(bar, "/ needle") {
		t.Errorf("the committed query is gone: %q", bar)
	}
	m = m.ClearSearch()
	if m.Searching() || strings.Contains(strings.Join(screen(m), "\n"), "/ needle") {
		t.Errorf("ClearSearch left the search:\n%s", strings.Join(screen(m), "\n"))
	}
}

func TestEmptyQueryClearsTheSearch(t *testing.T) {
	t.Parallel()

	m := feed(t, sized(text(50), 40, 10), tuitest.Press("/"), tuitest.Type("needle"), tuitest.Enter())
	m = feed(t, m, tuitest.Press("/"), tuitest.Press("backspace"), tuitest.Press("backspace"),
		tuitest.Press("backspace"), tuitest.Press("backspace"), tuitest.Press("backspace"), tuitest.Press("backspace"), tuitest.Enter())
	if m.Searching() {
		t.Error("an empty query left the search on")
	}
}

func TestPasteGoesToTheOpenPrompt(t *testing.T) {
	t.Parallel()

	m := feed(t, sized(text(50), 40, 10), tuitest.Paste("needle"))
	if m.Searching() || m.CapturesKeys() {
		t.Fatal("a paste with the prompt closed did something")
	}
	m = feed(t, m, tuitest.Press("/"), tuitest.Paste("needle"), tuitest.Enter())
	if bar := last(screen(m)); !strings.Contains(bar, "1/5") {
		t.Errorf("pasted query: %q", bar)
	}
}

func TestNewContentKeepsTheSearch(t *testing.T) {
	t.Parallel()

	m := feed(t, sized(text(50), 40, 10), tuitest.Press("/"), tuitest.Type("needle"), tuitest.Enter())
	m = m.SetContent(text(20))
	if bar := last(screen(m)); !strings.Contains(bar, "1/2") {
		t.Errorf("after new content: %q", bar)
	}
}

func TestHelpNamesTheKeys(t *testing.T) {
	t.Parallel()

	got := strings.Join(tuitest.HelpKeys(pager.New()), " ")
	for _, want := range []string{"/", "n/N"} {
		if !strings.Contains(got, want) {
			t.Errorf("help keys %q lack %q", got, want)
		}
	}
}

func last(lines []string) string { return lines[len(lines)-1] }
