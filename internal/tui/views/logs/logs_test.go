package logs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatLineSinglePodNoPrefix(t *testing.T) {
	t.Parallel()
	got := formatLine("nginx-7d-aaa", "hello world", false)
	if got != "hello world" {
		t.Errorf("single-pod line should pass through unchanged, got %q", got)
	}
}

func TestFormatLineMultiPodHasPrefix(t *testing.T) {
	t.Parallel()
	got := formatLine("nginx-7d-aaa", "hello world", true)
	if !strings.Contains(got, "nginx-7d-aaa") {
		t.Errorf("multi-pod line missing pod tag: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("multi-pod line missing message: %q", got)
	}
}

func TestPodColorIsDeterministic(t *testing.T) {
	t.Parallel()
	a1 := podColor("nginx-aaa")
	a2 := podColor("nginx-aaa")
	if a1 != a2 {
		t.Errorf("podColor not deterministic: %v vs %v", a1, a2)
	}
}

func TestPodColorBelongsToPalette(t *testing.T) {
	t.Parallel()
	got := podColor("anything")
	palette := []lipgloss.Color{
		"#7D56F4", "#04B575", "#F2A65A", "#FF6B9D", "#56B4D4",
		"#E5C07B", "#98C379", "#C678DD", "#56B6C2", "#E06C75",
	}
	for _, c := range palette {
		if got == c {
			return
		}
	}
	t.Errorf("podColor returned %v, not in palette", got)
}

func TestAppendLineEnforcesBufferCap(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", nil, 0)
	for i := 0; i < maxBufferedLines+50; i++ {
		m.appendLine("line")
	}
	if got := len(m.lines); got != maxBufferedLines {
		t.Errorf("buffer cap not enforced: got %d, want %d", got, maxBufferedLines)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	if err := m.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestNewWithNoClientClosesChannelImmediately(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	if _, ok := <-m.events; ok {
		t.Errorf("expected closed events channel when client is nil")
	}
}

func TestKubectlEquivalentSingleVsMulti(t *testing.T) {
	t.Parallel()

	single := New(nil, "demo", []string{"alpha"}, 200)
	if !strings.Contains(single.KubectlEquivalent(), "alpha") {
		t.Errorf("single-pod equivalent missing pod name: %q", single.KubectlEquivalent())
	}
	if !strings.Contains(single.KubectlEquivalent(), "--tail=200") {
		t.Errorf("single-pod equivalent missing tail value: %q", single.KubectlEquivalent())
	}

	multi := New(nil, "demo", []string{"alpha", "beta"}, 50)
	if !strings.Contains(multi.KubectlEquivalent(), "alpha,beta") {
		t.Errorf("multi-pod equivalent missing pod list: %q", multi.KubectlEquivalent())
	}
	if !strings.Contains(multi.KubectlEquivalent(), "--tail=50") {
		t.Errorf("multi-pod equivalent missing tail value: %q", multi.KubectlEquivalent())
	}
}

func TestNewClampsNonPositiveTailToDefault(t *testing.T) {
	t.Parallel()
	for _, in := range []int64{0, -1, -100} {
		m := New(nil, "ns", []string{"foo"}, in)
		if m.tail != defaultTailLines {
			t.Errorf("tail=%d → m.tail=%d, want %d", in, m.tail, defaultTailLines)
		}
	}
}

func TestComputeMatchesFindsSubstrings(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	m.appendLine("alpha line")
	m.appendLine("beta gamma")
	m.appendLine("alpha beta")
	m.appendLine("delta")

	m.searchQuery = "alpha"
	m.computeMatches()

	if len(m.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d (%v)", len(m.matches), m.matches)
	}
	if m.matches[0] != 0 || m.matches[1] != 2 {
		t.Errorf("match indices = %v, want [0 2]", m.matches)
	}
}

func TestComputeMatchesEmptyQueryClears(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	m.appendLine("alpha")
	m.matches = []int{0}
	m.searchQuery = ""
	m.computeMatches()
	if m.matches != nil {
		t.Errorf("empty query should clear matches, got %v", m.matches)
	}
}

func TestGotoMatchWrapsForwardsAndBackwards(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	m.matches = []int{0, 2, 5}
	m.matchIdx = 0

	m.gotoMatch(+1)
	if m.matchIdx != 1 {
		t.Errorf("after +1: idx=%d, want 1", m.matchIdx)
	}
	m.gotoMatch(+1)
	m.gotoMatch(+1)
	if m.matchIdx != 0 {
		t.Errorf("after wrap: idx=%d, want 0", m.matchIdx)
	}
	m.gotoMatch(-1)
	if m.matchIdx != 2 {
		t.Errorf("after -1 wrap: idx=%d, want 2", m.matchIdx)
	}
}

func TestGotoMatchPausesAutoFollow(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	m.matches = []int{0}
	m.autoFollow = true

	m.gotoMatch(+1)

	if m.autoFollow {
		t.Error("gotoMatch should pause auto-follow so the user can read the match")
	}
}

func TestGotoMatchOnEmptyMatchesIsNoOp(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100)
	m.matches = nil
	m.matchIdx = 0
	m.autoFollow = true

	m.gotoMatch(+1)

	if m.matchIdx != 0 {
		t.Errorf("idx changed on empty matches: %d", m.matchIdx)
	}
	if !m.autoFollow {
		t.Error("autoFollow flipped despite no-op")
	}
}

func TestCancelSearchClearsQueryButPreservesLogs(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100)
	m.appendLine("alpha tick")
	m.appendLine("beta tick")
	m.appendLine("gamma")

	m.searchQuery = "tick"
	m.computeMatches()
	if len(m.matches) != 2 {
		t.Fatalf("setup: expected 2 matches, got %d", len(m.matches))
	}

	m.cancelSearch()

	if m.searchQuery != "" {
		t.Errorf("query not cleared: %q", m.searchQuery)
	}
	if len(m.matches) != 0 {
		t.Errorf("matches not cleared: %v", m.matches)
	}
	if m.matchIdx != 0 {
		t.Errorf("matchIdx not reset: %d", m.matchIdx)
	}
	if len(m.lines) != 3 {
		t.Errorf("cancelSearch wiped log buffer (got %d lines, want 3)", len(m.lines))
	}
}

func TestCancelSearchPreservesAutoFollow(t *testing.T) {
	t.Parallel()

	for _, follow := range []bool{true, false} {
		m := New(nil, "ns", []string{"foo"}, 100)
		m.searchQuery = "x"
		m.autoFollow = follow

		m.cancelSearch()

		if m.autoFollow != follow {
			t.Errorf("autoFollow flipped: was %v, after cancelSearch %v", follow, m.autoFollow)
		}
	}
}

func TestClearLogsPreservesAutoFollow(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100)
	m.appendLine("a")
	m.appendLine("b")
	m.autoFollow = false // user paused

	m.clearLogs()

	if m.autoFollow {
		t.Error("clearLogs flipped autoFollow back to true — paused state must persist")
	}
	if len(m.lines) != 0 {
		t.Errorf("clearLogs left lines behind: %d", len(m.lines))
	}
}

func TestClearLogsLeavesSearchAlone(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100)
	m.searchQuery = "tick"

	m.clearLogs()

	if m.searchQuery != "tick" {
		t.Errorf("clearLogs touched search query: %q", m.searchQuery)
	}
}

func TestStatusLineCombinesPauseAndSearch(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100)
	m.appendLine("hello tick")
	m.searchQuery = "tick"
	m.computeMatches()
	m.autoFollow = false

	got := m.statusLine()

	if !strings.Contains(got, "paused") {
		t.Errorf("status line missing pause marker: %q", got)
	}
	if !strings.Contains(got, "/tick") {
		t.Errorf("status line missing query: %q", got)
	}
	if !strings.Contains(got, "1/1") {
		t.Errorf("status line missing match counter: %q", got)
	}
}

func TestStatusLineEmptyWhenIdle(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100)
	if got := m.statusLine(); got != "" {
		t.Errorf("idle status line should be empty, got %q", got)
	}
}

func TestHighlightContentReplacesMatches(t *testing.T) {
	t.Parallel()
	out := highlightContent("hello alpha world alpha", "alpha")
	// The query string itself must still appear at every match site (wrapped
	// in escape codes) — count the "alpha" occurrences.
	count := strings.Count(out, "alpha")
	if count != 2 {
		t.Errorf("expected 'alpha' to appear twice in output, got %d:\n%s", count, out)
	}
	// No-op for empty query.
	if got := highlightContent("hello", ""); got != "hello" {
		t.Errorf("empty query should pass content through, got %q", got)
	}
}
