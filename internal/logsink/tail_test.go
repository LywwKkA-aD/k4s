package logsink

import (
	"testing"
)

func TestTailBufferKeepsLastBytes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		limit         int
		writes        []string
		want          string
		wantTruncated bool
	}{
		{name: "empty", limit: 8, want: ""},
		{name: "under limit", limit: 8, writes: []string{"ab", "cd"}, want: "abcd"},
		{name: "exactly at limit", limit: 4, writes: []string{"ab", "cd"}, want: "abcd"},
		{name: "overflow drops oldest", limit: 4, writes: []string{"abc", "def"}, want: "cdef", wantTruncated: true},
		{name: "single write over limit", limit: 4, writes: []string{"abcdefgh"}, want: "efgh", wantTruncated: true},
		{name: "many small writes", limit: 3, writes: []string{"a", "b", "c", "d", "e"}, want: "cde", wantTruncated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newTailBuffer(tc.limit)
			for _, w := range tc.writes {
				n, err := b.Write([]byte(w))
				if err != nil || n != len(w) {
					t.Fatalf("Write(%q) = (%d, %v), want (%d, nil)", w, n, err, len(w))
				}
			}
			got, truncated := b.Snapshot()
			if string(got) != tc.want {
				t.Errorf("Snapshot() = %q, want %q", got, tc.want)
			}
			if truncated != tc.wantTruncated {
				t.Errorf("truncated = %v, want %v", truncated, tc.wantTruncated)
			}
		})
	}
}

func TestTailBufferSnapshotIsACopy(t *testing.T) {
	t.Parallel()

	b := newTailBuffer(8)
	_, _ = b.Write([]byte("abc"))
	got, _ := b.Snapshot()
	got[0] = 'X'
	again, _ := b.Snapshot()
	if string(again) != "abc" {
		t.Fatalf("Snapshot aliased the buffer: %q", again)
	}
}
