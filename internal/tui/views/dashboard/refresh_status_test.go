package dashboard_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// withoutPod is busyK3s without the problem pod named name.
func withoutPod(name string) func() dash.Snapshot {
	return func() dash.Snapshot {
		s := busyK3s()
		s.Pods.Problems = slices.DeleteFunc(slices.Clone(s.Pods.Problems), func(r dash.PodRow) bool {
			return r.Key.Name == name
		})
		return s
	}
}

// TestOneRefreshSendsOneStatus: a refresh that takes the row under the
// cursor away and brings a change outside the wide layout has two things
// for the status row, the note that the row is gone and the change's
// toast. One status row shows one, so the refresh sends one: the note,
// which answers where the user's cursor went.
func TestOneRefreshSendsOneStatus(t *testing.T) {
	t.Parallel()

	const gone = "payments-worker-6c9f7d8b5-xk2lp"
	cases := []struct {
		name string
		w, h int
	}{
		{name: "medium", w: 120, h: 34},
		{name: "compact", w: 80, h: 24},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFeed(busyK3s)
			m := mount(t, f, tc.w, tc.h)
			for range 6 {
				if strings.Contains(command(m), gone) {
					break
				}
				m, _ = press(t, m, "3", "down")
			}
			if !strings.Contains(command(m), gone) {
				t.Fatalf("the cursor is not on pod %s: %q", gone, command(m))
			}
			f.set(withoutPod(gone))
			_, out := tuitest.Drive(t, m, f.clock.Advance(tickStep)...)
			statuses := tuitest.All[views.StatusMsg](out)
			toasts := tuitest.All[views.ToastMsg](out)
			if len(statuses) != 1 || len(toasts) != 0 || statuses[0].Text != "pod shop/"+gone+" is gone" {
				t.Fatalf("one refresh sent %d statuses and %d toasts, want the gone note alone:\n%s", len(statuses), len(toasts), tuitest.Describe(out))
			}
		})
	}
}
