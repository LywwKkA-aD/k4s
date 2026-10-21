package tui_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// statusRow is the first footer row without its level mark (see
// TestStatusRowMarksItsLevel): the status message or the key hints.
func statusRow(footer string) string {
	row, _, _ := strings.Cut(footer, "\n")
	row = strings.TrimSpace(row)
	for _, mark := range []string{styles.GlyphOK, styles.GlyphWarn, styles.GlyphErr} {
		if rest, ok := strings.CutPrefix(row, mark+" "); ok {
			return rest
		}
	}
	return row
}

// A level is never shown by colour alone: OK, Warn and Error lead with the
// dashboard's glyph for that state, which mono and NO_COLOR keep.
func TestStatusRowMarksItsLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		level status.Level
		want  string
	}{
		{name: "info", level: status.Info, want: "resolving logs"},
		{name: "ok", level: status.OK, want: styles.GlyphOK + " resolving logs"},
		{name: "warn", level: status.Warn, want: styles.GlyphWarn + " resolving logs"},
		{name: "error", level: status.Error, want: styles.GlyphErr + " resolving logs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.StatusMsg{Level: tc.level, Text: "resolving logs"})
			row, _, _ := strings.Cut(footer(m), "\n")
			if got := strings.TrimSpace(row); got != tc.want {
				t.Fatalf("status row = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusMsgShowsInTheFooter(t *testing.T) {
	t.Parallel()

	for _, level := range []status.Level{status.Info, status.OK, status.Warn, status.Error} {
		m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.StatusMsg{Level: level, Text: "service web has no selector"})
		if got := statusRow(footer(m)); got != "service web has no selector" {
			t.Errorf("level %d: status row = %q", level, got)
		}
		if !strings.Contains(footer(m), "kubectl get all -A") {
			t.Errorf("level %d: the status pushed the command out of the footer:\n%s", level, footer(m))
		}
	}
}

func TestStatusMessagesExpire(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		level   status.Level
		steps   []tea.Msg     // after the message is set
		advance time.Duration // then the clock moves on
		showing bool
	}{
		{name: "info inside the ttl", level: status.Info, advance: status.TTL - time.Second, showing: true},
		{name: "info after the ttl", level: status.Info, advance: status.TTL},
		{name: "warn after the ttl", level: status.Warn, advance: status.TTL},
		{name: "a key does not cut a warning short", level: status.Warn, steps: tuitest.Keys("x"), advance: time.Second, showing: true},
		{name: "error outlives the ttl", level: status.Error, advance: time.Hour, showing: true},
		{name: "error lasts the ttl despite a key", level: status.Error, steps: tuitest.Keys("x"), advance: time.Second, showing: true},
		{name: "error goes at the ttl after a key", level: status.Error, steps: tuitest.Keys("x"), advance: status.TTL},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			m, _ := tuitest.Drive(t, startedAt(t, tuitest.NewFakeClient(t), clk), views.StatusMsg{Level: tc.level, Text: "boom"})
			m, _ = tuitest.Drive(t, m, tc.steps...)
			clk.Advance(tc.advance)
			if got := statusRow(footer(m)) == "boom"; got != tc.showing {
				t.Fatalf("showing = %v, want %v; footer:\n%s", got, tc.showing, footer(m))
			}
		})
	}
}

func TestErrorGoesOnTheFirstKeyAfterTheTTL(t *testing.T) {
	t.Parallel()

	clk := newClock()
	m, _ := tuitest.Drive(t, startedAt(t, tuitest.NewFakeClient(t), clk), colon("nope")...)
	clk.Advance(time.Minute)
	if got := statusRow(footer(m)); got != "unknown command: nope" {
		t.Fatalf("error gone before any key: %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("x"))
	if got := statusRow(footer(m)); !strings.HasPrefix(got, "q quit") {
		t.Fatalf("status row after the key = %q, want the hints back", got)
	}
}

func TestOpeningTheCommandBarClearsTheStatus(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, nil), colon("nope")...)
	m, _ = tuitest.Drive(t, m, tuitest.Press(":"), tuitest.Esc())
	if got := statusRow(footer(m)); !strings.HasPrefix(got, "q quit") {
		t.Fatalf("status row = %q, want the hints", got)
	}
}

// TestStatusHidesTheHomeDirectory sets HOME, so it cannot run in parallel.
func TestStatusHidesTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	text := "load kubeconfig: open " + home + "/.kube/config: permission denied"
	m, _ := tuitest.Drive(t, started(t, nil), views.StatusMsg{Level: status.Error, Text: text})
	if got := statusRow(footer(m)); got != "load kubeconfig: open ~/.kube/config: permission denied" {
		t.Fatalf("status row = %q", got)
	}
}
