package tui

import "testing"

// TestCutRows: a popup taller than the body keeps its first rows, the
// last of them saying more was cut; one row stays as it is.
func TestCutRows(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{name: "fits", in: "a\nb", n: 2, want: "a\nb"},
		{name: "room to spare", in: "a", n: 3, want: "a"},
		{name: "cut", in: "a\nb\nc\nd", n: 3, want: "a\nb\n…"},
		{name: "one row", in: "a\nb", n: 1, want: "a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cutRows(tc.in, tc.n); got != tc.want {
				t.Errorf("cutRows(%q, %d) = %q, want %q", tc.in, tc.n, got, tc.want)
			}
		})
	}
}
