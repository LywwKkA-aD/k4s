package logs_test

// The Close tests measure process-wide goroutines, so they do not run in
// parallel. Every other test in this package is parallel and stays parked
// until these finish; what earlier tests left winding down is measured as
// a baseline first.

import (
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// floodLines is far more than the view's event buffer holds, so stream
// goroutines nobody drains block on send until Close cancels them.
const floodLines = 5000

// viewPkg prefixes every frame of the view under test.
const viewPkg = "internal/tui/views/logs."

// settledParked is tuitest.Parked(viewPkg) once it has stopped moving.
func settledParked() int {
	prev, same := tuitest.Parked(viewPkg), 0
	for deadline := time.Now().Add(2 * time.Second); same < 3 && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		cur := tuitest.Parked(viewPkg)
		if cur == prev {
			same++
			continue
		}
		prev, same = cur, 0
	}
	return prev
}

func TestCloseStopsStreamGoroutines(t *testing.T) {
	cases := []struct {
		name string
		pods []string
	}{
		{name: "single pod", pods: []string{tuitest.PodWeb1}},
		{name: "multi pod merge", pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := tuitest.NewFakeClient(t, tuitest.WithLogs(strings.Repeat("line\n", floodLines)))
			before := settledParked()

			v := logs.New(c, tuitest.NamespaceDemo, tc.pods, 100, "")
			t.Cleanup(func() { _ = v.Close() })
			tuitest.Eventually(t, 2*time.Second, func() bool { return tuitest.Parked(viewPkg) > before },
				"the streams never blocked on the undrained view; the test proves nothing")

			if err := v.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for tuitest.Parked(viewPkg) > before {
				if time.Now().After(deadline) {
					t.Fatalf("%d stream goroutines survived Close (%d before):\n%s",
						tuitest.Parked(viewPkg), before, tuitest.Stacks(viewPkg))
				}
				time.Sleep(10 * time.Millisecond)
			}
			if err := v.Close(); err != nil {
				t.Errorf("second Close: %v", err)
			}
		})
	}
}

func TestStreamsEndWhenLogsEnd(t *testing.T) {
	c := tuitest.NewFakeClient(t, tuitest.WithLogs("one\ntwo\n"))
	base := tuitest.SettledGoroutines()

	v, out := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 100, ""), 100, 20)
	t.Cleanup(func() { _ = v.Close() })
	if screen := tuitest.Screen(v); !strings.Contains(screen, "two") || !strings.Contains(screen, "stream closed") {
		t.Errorf("finished stream screen:\n%s\nmessages:\n%s", screen, tuitest.Describe(out))
	}
	tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
	if err := v.Close(); err != nil {
		t.Errorf("Close after the streams ended: %v", err)
	}
}
