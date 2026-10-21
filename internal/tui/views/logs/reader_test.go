package logs

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// readAll drains r through a lineReader and returns the text of every line.
func readAll(t *testing.T, r io.Reader) []string {
	t.Helper()
	lr := newLineReader(r)
	var out []string
	for {
		line, err := lr.next()
		if line.ok {
			out = append(out, line.text())
		}
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("next: %v", err)
		}
	}
}

func TestLineReaderSplitsAndSanitizes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "plain lines", in: "one\ntwo\n", want: []string{"one", "two"}},
		{name: "last line without a line feed", in: "one\ntwo", want: []string{"one", "two"}},
		{name: "blank lines stay", in: "a\n\nb\n", want: []string{"a", "", "b"}},
		{name: "crlf", in: "a\r\nb\r\n", want: []string{"a", "b"}},
		{name: "application colours dropped", in: "\x1b[31mred\x1b[0m text\n", want: []string{"red text"}},
		{name: "osc 52 is inert", in: "copy\x1b]52;c;ZXZpbA==\amine\n", want: []string{"copymine"}},
		{name: "clear screen is inert", in: "\x1b[2J\x1b[Hhome\n", want: []string{"home"}},
		{name: "invalid utf-8", in: "a\xffb\n", want: []string{"a�b"}},
		{name: "empty stream", in: "", want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := readAll(t, strings.NewReader(tc.in))
			if strings.Join(got, "|") != strings.Join(tc.want, "|") || len(got) != len(tc.want) {
				t.Fatalf("lines = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLineReaderKeepsAHugeLineBoundedAndGoesOn(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("x", 2<<20)
	got := readAll(t, strings.NewReader(huge+"\nafter\n"))
	if len(got) != 2 || got[1] != "after" {
		t.Fatalf("got %d lines, last %q; want the huge line, then after", len(got), got[len(got)-1])
	}
	want := strings.Repeat("x", sanitize.MaxLine) + "…[+2031616 bytes]"
	if got[0] != want {
		t.Fatalf("huge line = %d bytes ending %q, want %d bytes ending %q",
			len(got[0]), got[0][len(got[0])-20:], len(want), want[len(want)-20:])
	}
}

func TestLineReaderCutsAHugeLineOnARune(t *testing.T) {
	t.Parallel()

	// "é" is two bytes: one of them straddles the cap.
	huge := "a" + strings.Repeat("é", sanitize.MaxLine)
	got := readAll(t, strings.NewReader(huge+"\n"))
	if len(got) != 1 || !utf8.ValidString(got[0]) || strings.ContainsRune(got[0], utf8.RuneError) {
		t.Fatalf("cut line is not clean UTF-8: %q…", got[0][:20])
	}
	if !strings.HasSuffix(got[0], "…[+65538 bytes]") {
		t.Fatalf("note = %q", got[0][len(got[0])-20:])
	}
}

func TestRawLineTimestamp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		in     string
		wantTS time.Time
		body   string
	}{
		{name: "rfc3339nano", in: "2026-08-24T12:00:00.123456789Z hello world", wantTS: time.Date(2026, 8, 24, 12, 0, 0, 123456789, time.UTC), body: "hello world"},
		{name: "no prefix", in: "hello world", body: "hello world"},
		{name: "space first", in: " hello", body: " hello"},
		{name: "not a time", in: "hello world", body: "hello world"},
		{name: "escape after the stamp", in: "2026-08-24T12:00:00Z \x1b[1mbold", wantTS: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), body: "bold"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ts, rest := rawLine{head: []byte(tc.in), ok: true}.withoutTimestamp()
			if !ts.Equal(tc.wantTS) || rest.text() != tc.body {
				t.Fatalf("withoutTimestamp = %v, %q; want %v, %q", ts, rest.text(), tc.wantTS, tc.body)
			}
		})
	}
}

func FuzzLineReader(f *testing.F) {
	for _, seed := range []string{"a\nb", "\x1b]52;c;x\a\n", "\xff\xfe\r\n", "\x9b31m\x9d0;t\x9c", strings.Repeat("y", 70000)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		lr := newLineReader(strings.NewReader(in))
		for {
			line, err := lr.next()
			if line.ok {
				text := line.text()
				if !inert(text) {
					t.Fatalf("unsafe line %q from %q", text, in)
				}
			}
			if err != nil {
				return
			}
		}
	})
}

// inert reports whether s is valid UTF-8 without C0 controls (tab aside),
// DEL or C1 controls: nothing a terminal would act on.
func inert(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 && r != '\t') || (r >= 0x7f && r <= 0x9f) {
			return false
		}
	}
	return true
}
