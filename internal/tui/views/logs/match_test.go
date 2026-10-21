package logs

import (
	"slices"
	"strings"
	"testing"
)

func TestQueryFindIsSmartCase(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		query string
		text  string
		want  []span
	}{
		{name: "lower case matches any case", query: "error", text: "ERROR here, error there, Error", want: []span{{0, 5}, {12, 17}, {25, 30}}},
		{name: "an upper-case letter makes it exact", query: "Error", text: "ERROR error Error", want: []span{{12, 17}}},
		{name: "all upper is exact too", query: "ERROR", text: "error ERROR", want: []span{{6, 11}}},
		{name: "occurrences do not overlap", query: "aa", text: "aaaaa", want: []span{{0, 2}, {2, 4}}},
		{name: "digits", query: "38", text: "status=38 code 381", want: []span{{7, 9}, {15, 17}}},
		{name: "no match", query: "zzz", text: "abc"},
		{name: "empty query matches nothing", query: "", text: "abc"},
		{name: "non-ascii folds", query: "é", text: "CAFÉ café", want: []span{{3, 5}, {9, 11}}},
		{name: "folds across byte lengths", query: "k", text: "aKb", want: []span{{1, 4}}},
		{name: "non-ascii text, ascii query", query: "ok", text: "日本 OK ok", want: []span{{7, 9}, {10, 12}}},
		{name: "invalid bytes never match", query: "a", text: "\xffa", want: []span{{1, 2}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := newQuery(tc.query).find(tc.text)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("find(%q, %q) = %v, want %v", tc.query, tc.text, got, tc.want)
			}
			for _, s := range got {
				if !strings.EqualFold(tc.text[s.start:s.end], tc.query) {
					t.Errorf("span %v is %q, not the query", s, tc.text[s.start:s.end])
				}
			}
		})
	}
}

func TestRowWithin(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		plain  string
		offset int
		width  int
		want   int
	}{
		{name: "first row", plain: "aaaaaa bbbbbb cccccc", offset: 2, width: 10, want: 0},
		{name: "second row", plain: "aaaaaa bbbbbb cccccc", offset: 8, width: 10, want: 1},
		{name: "third row", plain: "aaaaaa bbbbbb cccccc", offset: 14, width: 10, want: 2},
		{name: "on a dropped space", plain: "aaaaaa bbbbbb cccccc", offset: 6, width: 10, want: 1},
		{name: "a long word breaks hard", plain: strings.Repeat("x", 35) + "needle", offset: 35, width: 10, want: 3},
		{name: "wide characters", plain: "ab 日本語日本語日本語 cd", offset: len("ab 日本語日本語日本"), width: 10, want: 2},
		{name: "no width means one row", plain: "abc", offset: 2, width: 0, want: 0},
		{name: "past the end lands on the last row", plain: "aaaaaa bbbbbb", offset: 99, width: 10, want: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := rowWithin(tc.plain, tc.offset, tc.width); got != tc.want {
				t.Fatalf("rowWithin(%q, %d, %d) = %d, want %d", tc.plain, tc.offset, tc.width, got, tc.want)
			}
		})
	}
}
