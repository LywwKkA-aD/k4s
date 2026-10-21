package views_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestClampTail(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tail int64
		pods int
		cap  int64
		want int64
	}{
		{name: "one pod under the cap", tail: 100, pods: 1, cap: 5000, want: 100},
		{name: "one pod over the cap", tail: 100000, pods: 1, cap: 5000, want: 5000},
		{name: "twenty pods share the buffer", tail: 100000, pods: 20, cap: 250, want: 250},
		{name: "rounded up", tail: 100000, pods: 3, cap: 1667, want: 1667},
		{name: "small tail stays", tail: 50, pods: 20, cap: 250, want: 50},
		{name: "no pods count as one", tail: 9000, pods: 0, cap: 5000, want: 5000},
		{name: "more pods than lines", tail: 10, pods: 6000, cap: 1, want: 1},
		{name: "whole log is capped too", tail: 0, pods: 2, cap: 2500, want: 2500},
		{name: "negative is the whole log", tail: -1, pods: 4, cap: 1250, want: 1250},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := views.TailCap(tc.pods); got != tc.cap {
				t.Errorf("TailCap(%d) = %d, want %d", tc.pods, got, tc.cap)
			}
			if got := views.ClampTail(tc.tail, tc.pods); got != tc.want {
				t.Errorf("ClampTail(%d, %d) = %d, want %d", tc.tail, tc.pods, got, tc.want)
			}
		})
	}
}
