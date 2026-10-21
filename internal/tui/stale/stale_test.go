package stale_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/stale"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

var (
	errDown = errors.New("list pods: connection refused")
	errAuth = errors.New("list pods: forbidden")
)

// fold runs replies through a Health, reporting the status each emitted.
func fold(t *testing.T, replies ...error) (stale.Health, [][]views.StatusMsg) {
	t.Helper()
	var h stale.Health
	out := make([][]views.StatusMsg, 0, len(replies))
	for _, err := range replies {
		var cmd tea.Cmd
		h, cmd = h.Fold(err)
		out = append(out, tuitest.All[views.StatusMsg](tuitest.Collect(t, cmd)))
	}
	return h, out
}

func TestFoldAnnouncesEachNewErrorOnce(t *testing.T) {
	t.Parallel()

	h, got := fold(t, nil, errDown, errDown, errAuth, errAuth, nil, errAuth)
	want := [][]views.StatusMsg{
		nil,
		{{Level: status.Error, Text: errDown.Error()}},
		nil,
		{{Level: status.Error, Text: errAuth.Error()}},
		nil,
		nil,
		{{Level: status.Error, Text: errAuth.Error()}}, // same text, but after a recovery
	}
	for i := range want {
		if len(got[i]) != len(want[i]) || (len(want[i]) == 1 && got[i][0] != want[i][0]) {
			t.Errorf("reply %d emitted %v, want %v", i, got[i], want[i])
		}
	}
	if !errors.Is(h.Err(), errAuth) {
		t.Errorf("Err = %v, want the last error", h.Err())
	}
}

func TestStaleNeedsAGoodListBefore(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		replies    []error
		wantStale  bool
		wantFailed bool
	}{
		{name: "nothing yet"},
		{name: "ok", replies: []error{nil}},
		{name: "error before any list", replies: []error{errDown}, wantFailed: true},
		{name: "error after a list", replies: []error{nil, errDown}, wantStale: true},
		{name: "recovered", replies: []error{nil, errDown, nil}},
		{name: "errors after a recovery", replies: []error{errDown, nil, errDown}, wantStale: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := fold(t, tc.replies...)
			if h.Stale() != tc.wantStale || h.Failed() != tc.wantFailed {
				t.Errorf("Stale = %v, Failed = %v; want %v, %v", h.Stale(), h.Failed(), tc.wantStale, tc.wantFailed)
			}
		})
	}
}

func TestBanner(t *testing.T) {
	t.Parallel()

	long := errors.New("list pods: " + strings.Repeat("x", 200))
	cases := []struct {
		name    string
		err     error
		want    string
		maxLine int
	}{
		{name: "context prefix dropped", err: errDown, want: "stale: connection refused · r retry"},
		{name: "first line only", err: errors.New("list pods: a\nb"), want: "stale: a · r retry"},
		{name: "no list prefix kept as is", err: errors.New("Get \"https://x\": EOF"), want: "stale: Get \"https://x\": EOF · r retry"},
		{name: "long text is cut", err: long, maxLine: 90},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var h stale.Health
			h, _ = h.Fold(nil)
			h, _ = h.Fold(tc.err)
			got := tuitest.Strip(h.Banner())
			if tc.want != "" && got != tc.want {
				t.Errorf("Banner = %q, want %q", got, tc.want)
			}
			if tc.maxLine > 0 && (len([]rune(got)) > tc.maxLine || !strings.Contains(got, "…")) {
				t.Errorf("Banner = %q (%d runes), want one cut line of at most %d", got, len([]rune(got)), tc.maxLine)
			}
		})
	}
}

func TestBannerIsEmptyWhenNotStale(t *testing.T) {
	t.Parallel()

	var h stale.Health
	if h.Banner() != "" {
		t.Error("a fresh Health has a banner")
	}
	h, _ = h.Fold(errDown)
	if h.Banner() != "" {
		t.Error("an error before any list is the body, not a banner")
	}
}

func TestReset(t *testing.T) {
	t.Parallel()

	var h stale.Health
	h, _ = h.Fold(nil)
	h, _ = h.Fold(errDown)
	h = h.Reset()
	if h.Stale() || h.Failed() || h.Err() != nil {
		t.Errorf("Reset left %+v", h)
	}
	_, cmd := h.Fold(errDown)
	if got := tuitest.All[views.StatusMsg](tuitest.Collect(t, cmd)); len(got) != 1 {
		t.Errorf("an error after Reset emitted %v, want one status", got)
	}
}

func TestTrackSaysNothing(t *testing.T) {
	t.Parallel()

	var h stale.Health
	h = h.Track(nil)
	h = h.Track(errDown)
	if !h.Stale() || !errors.Is(h.Err(), errDown) {
		t.Errorf("Track(err) after a list: Stale = %v, Err = %v", h.Stale(), h.Err())
	}
	if _, cmd := h.Fold(errDown); cmd == nil {
		t.Error("Track used up the status Fold owes for a new error")
	}
}
