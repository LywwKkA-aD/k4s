package logs_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// The cluster writes what it likes to a log; these tests pin what reaches
// the operator's terminal.

func TestIngestKeepsTheStreamAliveAfterAHugeLine(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("x", 2<<20)
	for _, pods := range [][]string{{tuitest.PodWeb1}, {tuitest.PodWeb1, tuitest.PodWeb2}} {
		m := streaming(t, huge+"\nafter the flood\n", 20, pods...)
		screen := tuitest.Screen(m)
		if !strings.Contains(screen, "after the flood") || strings.Contains(screen, "stream error") {
			t.Errorf("%d pod(s): the stream did not survive a 2 MiB line:\n%s", len(pods), screen)
		}
	}
}

func TestIngestRendersEscapesInert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		body   string
		shows  string
		absent []string
	}{
		{name: "osc 52 clipboard", body: "copy\x1b]52;c;ZXZpbA==\amine\n", shows: "copymine", absent: []string{"\x1b]", "52;c;", "\a"}},
		{name: "clear screen and home", body: "\x1b[2J\x1b[Hhome\n", shows: "home", absent: []string{"\x1b[2J", "\x1b[H"}},
		{name: "application colour", body: "\x1b[31mred\x1b[0m text\n", shows: "red text", absent: []string{"\x1b[31m"}},
		{name: "title change", body: "\x1b]0;pwned\x07title\n", shows: "title", absent: []string{"pwned", "\x1b]0"}},
		{name: "carriage return overwrite", body: "fake prompt\rreal\n", shows: "fake promptreal", absent: []string{"\r"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, pods := range [][]string{{tuitest.PodWeb1}, {tuitest.PodWeb1, tuitest.PodWeb2}} {
				m := streaming(t, tc.body, 20, pods...)
				raw := m.View()
				if !strings.Contains(tuitest.Screen(m), tc.shows) {
					t.Errorf("%d pod(s): screen lacks %q:\n%s", len(pods), tc.shows, tuitest.Screen(m))
				}
				for _, a := range tc.absent {
					if strings.Contains(raw, a) {
						t.Errorf("%d pod(s): view carries %q from the log: %q", len(pods), a, raw)
					}
				}
			}
		})
	}
}
