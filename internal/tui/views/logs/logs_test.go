package logs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatRawLineSinglePodHasNoPrefix(t *testing.T) {
	t.Parallel()
	got := formatRawLine(logLine{pod: "nginx", text: "hello", kind: lineLog}, false, false)
	if got != "hello" {
		t.Errorf("single-pod line should pass through unchanged, got %q", got)
	}
}

func TestFormatRawLineCompactDoesNotEmbedPodName(t *testing.T) {
	t.Parallel()
	got := formatRawLine(logLine{pod: "nginx-aaa", text: "hello", kind: lineLog}, true, false)
	if strings.Contains(got, "nginx-aaa") {
		t.Errorf("compact prefix must not include the pod name, got %q", got)
	}
	if !strings.Contains(got, compactPrefix) {
		t.Errorf("compact prefix should include the bar glyph, got %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("compact prefix should keep the body, got %q", got)
	}
}

func TestFormatRawLineFullEmbedsPodName(t *testing.T) {
	t.Parallel()
	got := formatRawLine(logLine{pod: "nginx-aaa", text: "hello", kind: lineLog}, true, true)
	if !strings.Contains(got, "nginx-aaa") {
		t.Errorf("full prefix should include the pod name, got %q", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("full prefix should keep the body, got %q", got)
	}
}

func TestFormatRawLineStreamErrorAlwaysShowsPodName(t *testing.T) {
	t.Parallel()

	// Even in compact mode an error message must surface the pod name —
	// you can't read "stream error" without knowing which stream.
	got := formatRawLine(logLine{pod: "nginx-aaa", text: "boom", kind: lineStreamErr}, true, false)
	if !strings.Contains(got, "nginx-aaa") {
		t.Errorf("compact-mode stream error should still name the pod, got %q", got)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("error message missing: %q", got)
	}
}

func TestFormatRawLineStreamDoneAlwaysShowsPodName(t *testing.T) {
	t.Parallel()

	got := formatRawLine(logLine{pod: "nginx-aaa", kind: lineStreamDone}, true, false)
	if !strings.Contains(got, "nginx-aaa") {
		t.Errorf("compact-mode stream-closed should still name the pod, got %q", got)
	}
	if !strings.Contains(got, "stream closed") {
		t.Errorf("stream closed marker missing: %q", got)
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

func TestAppendRawEnforcesBufferCap(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", nil, 0, "")
	for i := 0; i < maxBufferedLines+50; i++ {
		m.appendRaw("foo", "line", lineLog)
	}
	if got := len(m.rawLines); got != maxBufferedLines {
		t.Errorf("buffer cap not enforced: got %d, want %d", got, maxBufferedLines)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
	if err := m.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestNewWithNoClientClosesChannelImmediately(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
	if _, ok := <-m.events; ok {
		t.Errorf("expected closed events channel when client is nil")
	}
}

func TestKubectlEquivalentSingleVsMulti(t *testing.T) {
	t.Parallel()

	single := New(nil, "demo", []string{"alpha"}, 200, "")
	if !strings.Contains(single.KubectlEquivalent(), "alpha") {
		t.Errorf("single-pod equivalent missing pod name: %q", single.KubectlEquivalent())
	}
	if !strings.Contains(single.KubectlEquivalent(), "--tail=200") {
		t.Errorf("single-pod equivalent missing tail value: %q", single.KubectlEquivalent())
	}

	multi := New(nil, "demo", []string{"alpha", "beta"}, 50, "")
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
		m := New(nil, "ns", []string{"foo"}, in, "")
		if m.tail != defaultTailLines {
			t.Errorf("tail=%d → m.tail=%d, want %d", in, m.tail, defaultTailLines)
		}
	}
}

func TestComputeMatchesOnRawTextNotPrefix(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"alpha", "beta"}, 100, "")
	m.appendRaw("alpha", "tick #1", lineLog)
	m.appendRaw("beta", "tock #1", lineLog)
	m.appendRaw("alpha", "tick #2", lineLog)

	// Searching for "alpha" should NOT match the bare log line text — the
	// pod name lives in the prefix, which the search must ignore.
	m.searchQuery = "alpha"
	m.computeMatches()
	if len(m.matches) != 0 {
		t.Errorf("search on pod name (in prefix) leaked into matches: %v", m.matches)
	}

	// Searching for body text should still work.
	m.searchQuery = "tick"
	m.computeMatches()
	if len(m.matches) != 2 {
		t.Errorf("expected 2 matches for 'tick', got %d (%v)", len(m.matches), m.matches)
	}
}

func TestComputeMatchesEmptyQueryClears(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.appendRaw("foo", "alpha", lineLog)
	m.matches = []int{0}
	m.searchQuery = ""
	m.computeMatches()
	if m.matches != nil {
		t.Errorf("empty query should clear matches, got %v", m.matches)
	}
}

func TestGotoMatchWrapsForwardsAndBackwards(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
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
	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.matches = []int{0}
	m.autoFollow = true
	m.gotoMatch(+1)
	if m.autoFollow {
		t.Error("gotoMatch should pause auto-follow so the user can read the match")
	}
}

func TestGotoMatchOnEmptyMatchesIsNoOp(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
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

func TestHighlightContentReplacesMatches(t *testing.T) {
	t.Parallel()
	out := highlightContent("hello alpha world alpha", "alpha")
	if count := strings.Count(out, "alpha"); count != 2 {
		t.Errorf("expected 'alpha' to appear twice in output, got %d:\n%s", count, out)
	}
	if got := highlightContent("hello", ""); got != "hello" {
		t.Errorf("empty query should pass content through, got %q", got)
	}
}

func TestCancelSearchClearsQueryButPreservesLogs(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.appendRaw("foo", "alpha tick", lineLog)
	m.appendRaw("foo", "beta tick", lineLog)
	m.appendRaw("foo", "gamma", lineLog)

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
	if len(m.rawLines) != 3 {
		t.Errorf("cancelSearch wiped log buffer (got %d lines, want 3)", len(m.rawLines))
	}
}

func TestCancelSearchPreservesAutoFollow(t *testing.T) {
	t.Parallel()
	for _, follow := range []bool{true, false} {
		m := New(nil, "ns", []string{"foo"}, 100, "")
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
	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.appendRaw("foo", "a", lineLog)
	m.appendRaw("foo", "b", lineLog)
	m.autoFollow = false
	m.clearLogs()
	if m.autoFollow {
		t.Error("clearLogs flipped autoFollow back to true — paused state must persist")
	}
	if len(m.rawLines) != 0 {
		t.Errorf("clearLogs left lines behind: %d", len(m.rawLines))
	}
}

func TestClearLogsLeavesSearchAlone(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.searchQuery = "tick"
	m.clearLogs()
	if m.searchQuery != "tick" {
		t.Errorf("clearLogs touched search query: %q", m.searchQuery)
	}
}

func TestStatusLineCombinesPauseAndSearch(t *testing.T) {
	t.Parallel()

	m := New(nil, "ns", []string{"foo"}, 100, "")
	m.appendRaw("foo", "hello tick", lineLog)
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
	m := New(nil, "ns", []string{"foo"}, 100, "")
	if got := m.statusLine(); got != "" {
		t.Errorf("idle status line should be empty, got %q", got)
	}
}

func TestStatusLineMentionsTagsWhenOn(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"alpha", "beta"}, 100, "")
	m.showPodNames = true
	if !strings.Contains(m.statusLine(), "tags") {
		t.Errorf("status line should advertise tags-on state: %q", m.statusLine())
	}
}

func TestStatusLineHasNoTagsHintForSinglePod(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"only"}, 100, "")
	m.showPodNames = true // even if forced, single-pod view never shows tags
	if strings.Contains(m.statusLine(), "tags") {
		t.Errorf("single-pod status must not mention tags: %q", m.statusLine())
	}
}

func TestHelpAdvertisesTagsKeyOnlyForMultiPod(t *testing.T) {
	t.Parallel()
	single := New(nil, "ns", []string{"only"}, 100, "")
	for _, b := range single.Help() {
		if b.Help().Key == "t" {
			t.Errorf("single-pod help should not include 't'")
		}
	}
	multi := New(nil, "ns", []string{"alpha", "beta"}, 100, "")
	found := false
	for _, b := range multi.Help() {
		if b.Help().Key == "t" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("multi-pod help should include 't'")
	}
}
