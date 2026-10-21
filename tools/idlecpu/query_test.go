package main

import (
	"testing"
)

func TestResponderScan(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		chunks []string
		want   string
	}{
		{name: "no query", chunks: []string{"\x1b[2J\x1b[Hhello"}, want: ""},
		{name: "primary device attributes", chunks: []string{"\x1b[c"}, want: replyDA1},
		{name: "explicit zero parameter", chunks: []string{"x\x1b[0cy"}, want: replyDA1},
		{name: "background with BEL", chunks: []string{"\x1b]11;?\x07\x1b[c"}, want: replyBackgroundBEL + replyDA1},
		{name: "background with ST", chunks: []string{"\x1b]11;?\x1b\\"}, want: replyBackgroundST},
		{name: "cursor position", chunks: []string{"\x1b[6n"}, want: replyCPR},
		{name: "extended cursor position is not answered", chunks: []string{"\x1b[?6n"}, want: ""},
		{name: "split across reads", chunks: []string{"abc\x1b]1", "1;?", "\x07def"}, want: replyBackgroundBEL},
		{name: "answered once when the carry holds it", chunks: []string{"\x1b[c", "plain"}, want: replyDA1},
		{name: "two queries in one read", chunks: []string{"\x1b[c\x1b[c"}, want: replyDA1 + replyDA1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := newResponder(terminalQueries)
			var got []byte
			for _, c := range tc.chunks {
				var reply []byte
				r, reply = r.scan([]byte(c))
				got = append(got, reply...)
			}
			if string(got) != tc.want {
				t.Fatalf("replies = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResponderCarryIsBounded(t *testing.T) {
	t.Parallel()

	r := newResponder(terminalQueries)
	big := make([]byte, 64*1024)
	r, _ = r.scan(big)
	if len(r.carry) >= r.longest {
		t.Fatalf("carry holds %d bytes, want fewer than the longest query (%d)", len(r.carry), r.longest)
	}
}
