package logs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// A pod name heads its lines in a multi-pod view and its notices and
// stream errors everywhere; it is drawn inert there, and painted lines
// keep the cleaned prefix.
func TestPodPrefixIsInert(t *testing.T) {
	t.Parallel()

	pod := tuitest.Hostile("api-7d9f-k7x2p")
	cases := []struct {
		name string
		kind lineKind
		full bool
		want string
	}{
		{name: "full prefix", kind: lineLog, full: true, want: "[api-7d9f-k7x2p] hello"},
		{name: "notice", kind: lineNotice, want: "[api-7d9f-k7x2p] hello"},
		{name: "alert", kind: lineAlert, want: "[api-7d9f-k7x2p] hello"},
		{name: "stream error", kind: lineStreamErr, want: "[api-7d9f-k7x2p] stream error: hello"},
	}
	p := newPainter()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ln := logLine{pod: pod, text: "hello", kind: tc.kind}
			rendered := formatRawLine(ln, true, tc.full, "#7D56F4")
			painted := p.line(ln, rendered, []span{{0, 2}}, noCurrent)
			for _, s := range []string{rendered, painted} {
				tuitest.AssertInert(t, s)
				if got := ansi.Strip(s); !strings.Contains(got, tc.want) {
					t.Errorf("line = %q, want %q", got, tc.want)
				}
			}
		})
	}
}
