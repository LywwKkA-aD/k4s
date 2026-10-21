package logs

import (
	"io"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// The re-resolve a tick starts covers every end before it: the model the
// tick returns says so, whatever order a return statement evaluates in.
func TestAReresolveCoversTheEndsBeforeIt(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	next, ok := r.m.streams.ended("web-h1-a", r.m.streams.pods["web-h1-a"].attempt, io.ErrUnexpectedEOF)
	if !ok {
		t.Fatal("the stream did not end")
	}
	r.m.streams = next
	ticks := r.clock.Advance(followInterval)
	if len(ticks) != 1 {
		t.Fatalf("%d timers went off, want the re-resolve's tick", len(ticks))
	}
	v, cmd := r.m.Update(ticks[0])
	m := v.(Model)
	if cmd == nil || !m.follow.Busy() {
		t.Fatal("the tick started no re-resolve")
	}
	if m.explain.upTo != m.streams.ends || m.streams.ends == 0 {
		t.Fatalf("the re-resolve covers ends up to %d, %d ended", m.explain.upTo, m.streams.ends)
	}
}
