package forwards

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// Names come from the state file and errors from the cluster: every cell
// of a row is inert, and a tab cannot widen a cell after it was measured.
func TestRowForIsInert(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	fwd := forwards.Forward{
		Kind: tuitest.Hostile("pod"), Namespace: tuitest.Hostile("shop"), Name: tuitest.Hostile("web-1"),
		LocalPort: 8080, RemotePort: 80,
	}
	cases := []struct {
		name string
		a    forwards.Active
		want []string
	}{
		{
			name: "a tunnel error",
			a:    forwards.Active{Forward: fwd, TunnelErr: "connection\trefused " + tuitest.Hostile("by web"), TunnelErrAt: at},
			want: []string{"pod", "shop", "web-1", "8080 → 80", "last error: connection refused by web · 3s ago"},
		},
		{
			name: "a failed forward",
			a:    forwards.Active{Forward: fwd, Status: forwards.StatusError, Err: errors.New("pods\t" + tuitest.Hostile("web-1") + " not found")},
			want: []string{"pod", "shop", "web-1", "pods web-1 not found"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			row := rowFor(tc.a, at.Add(3*time.Second))
			joined := strings.Join(row, "|")
			tuitest.AssertInert(t, joined)
			if strings.ContainsRune(joined, '\t') {
				t.Errorf("row keeps a tab: %q", joined)
			}
			for _, w := range tc.want {
				if !strings.Contains(joined, w) {
					t.Errorf("row %q lacks the cleaned %q", joined, w)
				}
			}
		})
	}
}
