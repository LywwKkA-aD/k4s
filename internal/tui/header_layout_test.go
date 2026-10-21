package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMiddleTruncate(t *testing.T) {
	t.Parallel()

	arn := "arn:aws:eks:eu-west-1:123456789012:cluster/payments"
	cases := []struct {
		name string
		s    string
		n    int
		want string
	}{
		{name: "fits", s: "default", n: 12, want: "default"},
		{name: "exact", s: "default", n: 7, want: "default"},
		{name: "keeps both ends", s: arn, n: 12, want: "arn:a…yments"},
		{name: "even room leans to the end", s: "abcdefghij", n: 6, want: "ab…hij"},
		{name: "one cell", s: arn, n: 1, want: "a"},
		{name: "no room", s: arn, n: 0, want: ""},
		{name: "wide characters count as two cells", s: "日本語のクラスター", n: 7, want: "日…ター"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := middleTruncate(tc.s, tc.n)
			if got != tc.want {
				t.Fatalf("middleTruncate(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.want)
			}
			if w := ansi.StringWidth(got); w > tc.n {
				t.Fatalf("%q is %d cells, over %d", got, w, tc.n)
			}
		})
	}
}
