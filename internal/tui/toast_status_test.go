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

// TestToastNeverHidesAnError: a toast is news nobody asked for, so it
// never replaces an error still on the status row, however long the error
// has waited for its key. Anything else it replaces, and once the error
// is gone it shows.
func TestToastNeverHidesAnError(t *testing.T) {
	t.Parallel()

	const toast = "pod shop/api → CrashLoopBackOff"
	cases := []struct {
		name    string
		before  []tea.Msg
		advance time.Duration // before the toast arrives
		want    string
	}{
		{name: "nothing showing", want: toast},
		{name: "an error showing", before: []tea.Msg{views.StatusMsg{Level: status.Error, Text: "describe: boom"}}, want: "describe: boom"},
		{name: "an error past its ttl, no key yet", before: []tea.Msg{views.StatusMsg{Level: status.Error, Text: "describe: boom"}}, advance: time.Hour, want: "describe: boom"},
		{name: "an error about a command", before: []tea.Msg{views.StatusAboutMsg{StatusMsg: views.StatusMsg{Level: status.Error, Text: "no shell"}}}, want: "no shell"},
		{name: "an error dismissed by a key", before: []tea.Msg{views.StatusMsg{Level: status.Error, Text: "describe: boom"}, tuitest.Press("x")}, advance: time.Hour, want: toast},
		{name: "a warning showing", before: []tea.Msg{views.StatusMsg{Level: status.Warn, Text: "0 replicas"}}, want: toast},
		{name: "an alert showing", before: []tea.Msg{views.ToastMsg{Level: status.Alert, Text: "pod shop/web → Error"}}, want: toast},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			m, _ := tuitest.Drive(t, startedAt(t, tuitest.NewFakeClient(t), clk), tc.before...)
			clk.Advance(tc.advance)
			m, _ = tuitest.Drive(t, m, views.ToastMsg{Level: status.Alert, Text: toast})
			if got := statusRow(footer(m)); got != tc.want {
				t.Fatalf("status row = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestToastShowsAsItsLevel: a toast shows with its level's glyph and goes
// at the TTL, an alert as well: it is coloured as an error but does not
// wait for a key.
func TestToastShowsAsItsLevel(t *testing.T) {
	t.Parallel()

	cases := []struct {
		level status.Level
		mark  string
	}{
		{level: status.Info},
		{level: status.OK, mark: styles.GlyphOK + " "},
		{level: status.Warn, mark: styles.GlyphWarn + " "},
		{level: status.Alert, mark: styles.GlyphErr + " "},
	}
	for _, tc := range cases {
		t.Run(tc.mark+"level", func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			m, _ := tuitest.Drive(t, startedAt(t, tuitest.NewFakeClient(t), clk), views.ToastMsg{Level: tc.level, Text: "pod shop/api → x"})
			row, _, _ := strings.Cut(footer(m), "\n")
			if got := strings.TrimSpace(row); got != tc.mark+"pod shop/api → x" {
				t.Fatalf("status row = %q", got)
			}
			clk.Advance(status.TTL)
			if got := statusRow(footer(m)); !strings.HasPrefix(got, "q quit") {
				t.Fatalf("status row at the ttl = %q, want the hints back", got)
			}
		})
	}
}
