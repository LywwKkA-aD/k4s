package logs_test

// The Close tests measure the process-wide goroutine count, so they do not
// run in parallel. Every other test in this package is parallel and stays
// parked until these finish.

import (
	"runtime"
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
			base := tuitest.SettledGoroutines()

			v := logs.New(c, tuitest.NamespaceDemo, tc.pods, 100, "")
			tuitest.Eventually(t, 2*time.Second, func() bool { return runtime.NumGoroutine() > base },
				"streams never started")
			if parked := tuitest.SettledGoroutines(); parked <= base || tuitest.Parked(viewPkg) == 0 {
				t.Fatalf("streams finished on their own (%d goroutines, base %d); the test proves nothing", parked, base)
			}

			if err := v.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
			if n := tuitest.Parked(viewPkg); n != 0 {
				t.Errorf("%d stream goroutines survived Close:\n%s", n, tuitest.Stacks(viewPkg))
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
	if screen := tuitest.Screen(v); !strings.Contains(screen, "two") || !strings.Contains(screen, "stream closed") {
		t.Errorf("finished stream screen:\n%s\nmessages:\n%s", screen, tuitest.Describe(out))
	}
	tuitest.WaitGoroutines(t, base, 5*time.Second, viewPkg)
	if err := v.Close(); err != nil {
		t.Errorf("Close after the streams ended: %v", err)
	}
}
