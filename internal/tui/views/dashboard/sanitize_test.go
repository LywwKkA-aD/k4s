package dashboard

import (
	"strings"
	"testing"
)

func TestClean(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain ascii is kept", in: "payments-worker-6c9f7d8b5-xk2lp", want: "payments-worker-6c9f7d8b5-xk2lp"},
		{name: "unicode text is kept", in: "Back-off · ×3 ≈ ▲", want: "Back-off · ×3 ≈ ▲"},
		{name: "osc 52 clipboard write is inert", in: "a\x1b]52;c;ZXZpbA==\x07b", want: "ab"},
		{name: "osc with string terminator", in: "a\x1b]0;title\x1b\\b", want: "ab"},
		{name: "csi colour and cursor moves", in: "\x1b[31mred\x1b[0m\x1b[2J\x1b[H", want: "red"},
		{name: "escape pair is a sequence too", in: "a\x1bcb\x07c", want: "abc"},
		{name: "newlines and tabs become spaces", in: "line one\nline\ttwo\r\n", want: "line one line two "},
		{name: "c1 csi opens a sequence, c1 and delete go", in: "a\u0085b\u009b1mc\x7fd", want: "abcd"},
		{name: "bidi overrides and isolates", in: "evil\u202egnp.exe\u2066x\u2069", want: "evilgnp.exex"},
		{name: "invalid utf-8", in: "a\xffb", want: "a\uFFFDb"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := clean(tt.in); got != tt.want {
				t.Errorf("clean(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCleanLeavesOneInertLine: whatever the cluster sends, a value is one
// line with no control a terminal could act on and no bidi control.
func TestCleanLeavesOneInertLine(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for r := rune(0); r < 0x2100; r++ {
		b.WriteRune(r)
	}
	for _, r := range clean(b.String()) {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			t.Fatalf("clean left control %U", r)
		}
	}
}

func TestCleanArgs(t *testing.T) {
	t.Parallel()

	in := []string{"describe", "pod", "x\x1b[2Jy", "-n", "shop"}
	got := cleanArgs(in)
	if strings.Join(got, " ") != "describe pod xy -n shop" {
		t.Fatalf("cleanArgs = %q", got)
	}
	if in[2] != "x\x1b[2Jy" {
		t.Fatal("cleanArgs changed its input")
	}
}
