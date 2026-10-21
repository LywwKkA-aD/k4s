package logs

import (
	"io"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// flaky ends pod's stream every 2s for a minute, whenever it is open,
// right after a line: the line resets the reconnect's backoff, so only the
// checks' own pace bounds what the drops cost.
func (r *rig) flaky(pod string) {
	r.t.Helper()
	for i := range 30 {
		if r.m.streams.pods[pod].phase == phaseLive {
			r.feed(r.line(pod, stamped(int64(2*i), 0), "tick"), r.end(pod, io.ErrUnexpectedEOF))
		}
		r.advance(2 * time.Second)
	}
}

// M5.7: an open logs view lists pods at most six times a minute, however
// often its streams drop. Ends wait for the next re-resolve.
func TestDropsCostAtMostSixListsAMinute(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	r.flaky("web-h1-a")
	if got := r.calls(); got["list pods"] > 6 || got["get pods"] != 0 {
		t.Fatalf("a minute of drops made %v, want at most 6 pod lists and no GET", got)
	}
	if r.src.count("web-h1-a") < 3 {
		t.Errorf("the stream reconnected %d times", r.src.count("web-h1-a")-1)
	}
}

// A view that does not follow its pods checks them at most six times a
// minute too: ends that come between two checks wait for the next one.
func TestDropsCostAtMostSixChecksAMinute(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.flaky("web-h1-a")
	if got := r.calls(); got["get pods"] > 6 || len(got) != 1 {
		t.Fatalf("a minute of drops made %v, want at most 6 GETs", got)
	}
	if r.src.count("web-h1-a") < 3 {
		t.Errorf("the stream reconnected %d times", r.src.count("web-h1-a")-1)
	}
}

// The first end after a quiet spell is still checked at once, and the
// check after it no sooner than checkSpacing later.
func TestChecksAreSpaced(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(explainDebounce)
	if got := r.calls()["get pods"]; got != 1 {
		t.Fatalf("the first end made %d GETs", got)
	}
	// The spacing runs from the check: checkSpacing - retryDelay(0) is left.
	r.advance(retryDelay(0))
	r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(checkSpacing - retryDelay(0) - time.Millisecond)
	if got := r.calls()["get pods"]; got != 1 {
		t.Fatalf("an end inside the spacing was checked early: %d GETs", got)
	}
	r.advance(time.Millisecond)
	if got := r.calls()["get pods"]; got != 2 {
		t.Fatalf("the end left waiting was not checked when the spacing ran out: %d GETs", got)
	}
	r.advance(time.Minute)
	if got := r.calls()["get pods"]; got != 2 || r.clock.Pending() != 0 {
		t.Errorf("a quiet minute made %d GETs and left %d timers", got, r.clock.Pending())
	}
}
