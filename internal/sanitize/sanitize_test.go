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
		{name: "bidi embeddings and overrides are dropped", in: "a\u202ab\u202bc\u202cd\u202de\u202ef", want: "abcdef"},
		{name: "bidi isolates are dropped", in: "evil\u2066x\u2067y\u2068z\u2069.exe", want: "evilxyz.exe"},
		{name: "zero-width and directional marks are dropped", in: "a\u200bb\u200cc\u200dd\u200ee\u200ff\u061cg", want: "abcdefg"},
		{name: "line and paragraph separators are dropped", in: "a\u2028b\u2029c", want: "abc"},
		{name: "invisible operators are dropped", in: "a\u2060b\u2061c\u2062d\u2063e\u2064f", want: "abcdef"},
		{name: "deprecated format characters are dropped", in: "a\u206ab\u206bc\u206cd\u206de\u206ef\u206fg", want: "abcdefg"},
		{name: "byte order mark and annotations are dropped", in: "\ufeffa\ufff9b\ufffac\ufffbd", want: "abcd"},
		{name: "tag characters are dropped", in: "a\U000e0000b\U000e0041c\U000e007fd", want: "abcd"},
		{name: "soft hyphen is dropped", in: "kube\u00adctl", want: "kubectl"},
		{name: "neighbours of the dropped ranges are kept", in: "\u00ac\u00ae\u061b\u200a\u2010\u2027\u2030\u205f\u2070\ufefc\ufffc\U000e0100", want: "\u00ac\u00ae\u061b\u200a\u2010\u2027\u2030\u205f\u2070\ufefc\ufffc\U000e0100"},
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
		{name: "bidi controls go on every line", in: "a\u202egnp.exe\nb\u2066c\u2069", want: "agnp.exe\nbc"},
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

// Not parallel: AllocsPerRun refuses to run alongside other tests.
func TestTextReturnsCleanInputUnchanged(t *testing.T) {
	in := "I1024 12:00:00 first line\n\tsecond line\n"
	allocs := testing.AllocsPerRun(100, func() { _ = Text(in) })
	if allocs != 0 {
		t.Fatalf("Text allocated %v times for clean input", allocs)
	}
}

func FuzzLine(f *testing.F) {
	for _, s := range []string{"", "plain", "\x1b[31mred", "\x1b]52;c;aGk=\a", "\u009b1m", "\xff\xfe", "a\r\nb\x00", "a\u200d\ufeff\U000e0041\u00adb"} {
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
		case r == 0xad, r == 0x61c, r == 0xfeff, r >= 0x200b && r <= 0x200f, r >= 0x2028 && r <= 0x202e,
			r >= 0x2060 && r <= 0x2064, r >= 0x2066 && r <= 0x206f, r >= 0xfff9 && r <= 0xfffb, r >= 0xe0000 && r <= 0xe007f:
			t.Fatalf("invisible format character %U in %q", r, out)
		}
	}
}
