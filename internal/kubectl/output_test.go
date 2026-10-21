package kubectl

import (
	"strings"
	"testing"
)

func TestFirstLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"blank lines first", "\n  \n\terror: boom\nmore", "error: boom"},
		{"colour codes", "\x1b[1;31merror\x1b[0m: boom", "error: boom"},
		{"osc title with bel", "\x1b]0;pwned\x07error", "error"},
		{"osc with string terminator", "\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\", "link"},
		{"lone escape", "a\x1bcb", "ab"},
		{"carriage return and bell", "error\a: boom\r", "error: boom"},
		{"invalid utf-8", "bad \xff byte", "bad  byte"},
		{"tabs survive", "a\tb", "a\tb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := firstLine([]byte(tt.in)); got != tt.want {
				t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestCappedBuffer(t *testing.T) {
	t.Parallel()

	b := &cappedBuffer{limit: 5}
	for _, chunk := range []string{"abc", "defg", "hij"} {
		n, err := b.Write([]byte(chunk))
		if err != nil || n != len(chunk) {
			t.Fatalf("Write(%q) = %d, %v; want %d, nil", chunk, n, err, len(chunk))
		}
	}
	if got := b.buf.String(); got != "abcde" {
		t.Errorf("kept %q, want %q", got, "abcde")
	}
	if !b.truncated {
		t.Error("truncated = false")
	}
	if strings.Contains(b.buf.String(), "f") {
		t.Error("kept bytes past the limit")
	}
}
