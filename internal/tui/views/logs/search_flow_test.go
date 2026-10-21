package logs_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

func search(t *testing.T, m logs.Model, query string) logs.Model {
	t.Helper()
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type(query), tuitest.Enter())
	return m
}

func TestSearchIsSmartCase(t *testing.T) {
	t.Parallel()

	body := "ERROR one\nerror two\nError three\nfine\n"
	cases := []struct {
		query string
		title string
	}{
		{query: "error", title: "logs · web-1 · /error · 1/3 · paused"},
		{query: "Error", title: "logs · web-1 · /Error · 1/1 · paused"},
		{query: "ERROR", title: "logs · web-1 · /ERROR · 1/1 · paused"},
		{query: "e", title: "logs · web-1 · /e · 1/7 · paused"},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			t.Parallel()
			m := search(t, streaming(t, body, 20, tuitest.PodWeb1), tc.query)
			requireTitle(t, m, tc.title)
		})
	}
}

func TestSearchCountsEveryOccurrence(t *testing.T) {
	t.Parallel()

	m := search(t, streaming(t, "alpha alpha\nbeta alpha\ngamma\n", 20, tuitest.PodWeb1), "alpha")
	requireTitle(t, m, "logs · web-1 · /alpha · 1/3 · paused")
	steps := []string{"2/3", "3/3", "1/3"}
	for _, want := range steps {
		m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
		requireTitle(t, m, "logs · web-1 · /alpha · "+want+" · paused")
	}
}

// sgrWellFormed reports the first escape in s that is not a complete SGR
// sequence ("\x1b[" params "m"), or "" when every one is.
func sgrWellFormed(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != 0x1b {
			continue
		}
		j := i + 1
		if j >= len(s) || s[j] != '[' {
			return fmt.Sprintf("ESC not followed by [ at %d: %q", i, s[i:min(i+12, len(s))])
		}
		j++
		for j < len(s) && (s[j] >= '0' && s[j] <= '9' || s[j] == ';' || s[j] == ':') {
			j++
		}
		if j >= len(s) || s[j] != 'm' {
			return fmt.Sprintf("broken SGR at %d: %q", i, s[i:min(i+16, len(s))])
		}
		i = j
	}
	return ""
}

func TestSearchNeverBreaksTheColours(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range 40 {
		fmt.Fprintf(&b, "req %d took 38ms code=512 m=%d\n", i, i%7)
	}
	body := b.String()
	for _, query := range []string{"0", "38", "12", "5", "m", ";", "["} {
		for _, keys := range [][]string{nil, {"t"}, {"w"}} {
			t.Run(fmt.Sprintf("%s after %v", query, keys), func(t *testing.T) {
				t.Parallel()
				m := streaming(t, body, 20, tuitest.PodWeb1, tuitest.PodWeb2)
				m, _ = tuitest.Drive(t, m, tuitest.Keys(keys...)...)
				m = search(t, m, query)
				m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
				for n, line := range strings.Split(m.View(), "\n") {
					if bad := sgrWellFormed(line); bad != "" {
						t.Fatalf("row %d: %s\n%q", n, bad, line)
					}
				}
			})
		}
	}
}

func TestSearchLandsInsideAWrappedLine(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range 60 {
		if i == 10 {
			b.WriteString("a short needle\n")
			continue
		}
		fmt.Fprintf(&b, "before %02d\n", i)
	}
	b.WriteString(strings.Repeat("filler ", 300) + "needle at the far end\n")
	for i := range 60 {
		fmt.Fprintf(&b, "after %02d\n", i)
	}
	c := tuitest.NewFakeClient(t, tuitest.WithLogs(b.String()))
	m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 500, ""), 40, 12)
	t.Cleanup(func() { _ = m.Close() })

	m, _ = tuitest.Drive(t, m, tuitest.Press("w"))
	m = search(t, m, "needle")
	steps := []struct {
		key, title, shows string
	}{
		{title: "logs · web-1 · /needle · 1/2 · paused", shows: "a short needle"},
		{key: "n", title: "logs · web-1 · /needle · 2/2 · paused", shows: "far end"},
		{key: "N", title: "logs · web-1 · /needle · 1/2 · paused", shows: "a short needle"},
		{key: "N", title: "logs · web-1 · /needle · 2/2 · paused", shows: "far end"},
	}
	for _, s := range steps {
		if s.key != "" {
			m, _ = tuitest.Drive(t, m, tuitest.Press(s.key))
		}
		requireTitle(t, m, s.title)
		if screen := tuitest.Screen(m); !strings.Contains(screen, s.shows) {
			t.Fatalf("after %q the match is off screen, want %q:\n%s", s.key, s.shows, screen)
		}
	}
}
