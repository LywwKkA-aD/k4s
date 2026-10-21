package sanitize

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestLine(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain text is unchanged", in: "GET /healthz 200", want: "GET /healthz 200"},
		{name: "tab is kept", in: "a\tb", want: "a\tb"},
		{name: "unicode is kept", in: "naïve ✓ 日本", want: "naïve ✓ 日本"},
		{name: "SGR colour is dropped", in: "\x1b[31mERROR\x1b[0m boom", want: "ERROR boom"},
		{name: "CSI with private params", in: "a\x1b[?25lb", want: "ab"},
		{name: "clear screen", in: "\x1b[2J\x1b[Hhi", want: "hi"},
		{name: "OSC 52 clipboard with BEL", in: "x\x1b]52;c;aGk=\ay", want: "xy"},
		{name: "OSC title with ST", in: "x\x1b]0;pwned\x1b\\y", want: "xy"},
		{name: "OSC 8 hyperlink keeps the text", in: "\x1b]8;;http://e.x\x1b\\link\x1b]8;;\x1b\\", want: "link"},
		{name: "DCS payload", in: "a\x1bPq#0;2;0;0;0\x1b\\b", want: "ab"},
		{name: "APC payload", in: "a\x1b_Gf=100;AAAA\x1b\\b", want: "ab"},
		{name: "two-byte escape", in: "a\x1bcb\x1b7c", want: "abc"},
		{name: "charset designation", in: "a\x1b(Bb", want: "ab"},
		{name: "lone ESC at the end", in: "abc\x1b", want: "abc"},
		{name: "unterminated CSI", in: "abc\x1b[31", want: "abc"},
		{name: "unterminated OSC eats the rest", in: "abc\x1b]52;c;aGk=", want: "abc"},
		{name: "C0 controls are dropped", in: "a\x00b\x07c\x08d\x0be\x0cf", want: "abcdef"},
		{name: "CR and LF are dropped", in: "progress 10%\rprogress 20%\n", want: "progress 10%progress 20%"},
		{name: "DEL is dropped", in: "a\x7fb", want: "ab"},
		{name: "C1 CSI rune starts a sequence", in: "a\u009b31mb", want: "ab"},
		{name: "C1 OSC rune with C1 ST", in: "a\u009d0;t\u009cb", want: "ab"},
		{name: "other C1 controls are dropped", in: "a\u0085b\u0080c", want: "abc"},
		{name: "invalid UTF-8 becomes U+FFFD", in: "a\xffb\xc3", want: "a�b�"},
		{name: "raw 8-bit CSI byte is invalid UTF-8", in: "a\x9b31mb", want: "a�31mb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Line(tc.in); got != tc.want {
				t.Fatalf("Line(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "lines are kept", in: "a\nb\n", want: "a\nb\n"},
		{name: "CRLF becomes LF", in: "a\r\nb", want: "a\nb"},
		{name: "escapes go on every line", in: "\x1b[1ma\x1b[0m\n\x1b]0;t\ab", want: "a\nb"},
		{name: "an unterminated OSC stops at the line end", in: "a\x1b]0;t\nb", want: "a\nb"},
		{name: "an unterminated CSI stops at the line end", in: "a\x1b[3\nb", want: "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Text(tc.in); got != tc.want {
				t.Fatalf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestLineCapsLength(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", MaxLine+100)
	got := Line(long)
	want := strings.Repeat("a", MaxLine) + "…[+100 bytes]"
	if got != want {
		t.Fatalf("Line(%d bytes) = %d bytes ending %q, want %d bytes", len(long), len(got), got[len(got)-20:], len(want))
	}

	// A multi-byte rune straddling the cap is not split.
	edge := strings.Repeat("a", MaxLine-1) + "é" + "b"
	got = Line(edge)
	if !utf8.ValidString(got) {
		t.Fatalf("cut through a rune: %q", got[len(got)-20:])
	}
	if !strings.HasSuffix(got, "…[+3 bytes]") {
		t.Fatalf("Line(edge) ends %q, want …[+3 bytes]", got[len(got)-20:])
	}

	if exact := strings.Repeat("a", MaxLine); Line(exact) != exact {
		t.Fatal("a line of exactly MaxLine bytes was cut")
	}
}

func TestTextCapsEachLine(t *testing.T) {
	t.Parallel()

	in := strings.Repeat("x", MaxLine+1) + "\nshort"
	got := Text(in)
	want := strings.Repeat("x", MaxLine) + "…[+1 bytes]\nshort"
	if got != want {
		t.Fatalf("Text did not cap the first line: got %d bytes", len(got))
	}
}

// Not parallel: AllocsPerRun refuses to run alongside other tests.
func TestLineReturnsCleanInputUnchanged(t *testing.T) {
	in := "plain ascii line with\ttab"
	allocs := testing.AllocsPerRun(100, func() { _ = Line(in) })
	if allocs != 0 {
		t.Fatalf("Line allocated %v times for clean input", allocs)
	}
}

func FuzzLine(f *testing.F) {
	for _, s := range []string{"", "plain", "\x1b[31mred", "\x1b]52;c;aGk=\a", "\u009b1m", "\xff\xfe", "a\r\nb\x00"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		checkInert(t, Line(in), false)
		checkInert(t, Text(in), true)
	})
}

func checkInert(t *testing.T, out string, newlines bool) {
	t.Helper()
	if !utf8.ValidString(out) {
		t.Fatalf("invalid UTF-8 in %q", out)
	}
	for _, r := range out {
		switch {
		case r == '\t':
		case r == '\n' && newlines:
		case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
			t.Fatalf("control %U in %q", r, out)
		}
	}
}
