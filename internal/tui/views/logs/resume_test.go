package logs

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

// replays serves each pod's streams in turn: a body, cut short when it
// ends in "|cut".
type replays struct {
	mu     sync.Mutex
	bodies map[string][]string
	asked  []k8s.LogOptions
}

func (r *replays) StreamLogs(_ context.Context, _, pod string, opts k8s.LogOptions) (io.ReadCloser, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.asked = append(r.asked, opts)
	body := r.bodies[pod][0]
	r.bodies[pod] = r.bodies[pod][1:]
	if cut, ok := strings.CutSuffix(body, "|cut"); ok {
		return io.NopCloser(cutStream{strings.NewReader(cut)}), nil
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func (r *replays) request(i int) k8s.LogOptions {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.asked[i]
}

// A stream cut mid-line and reconnected from its last line's second shows
// every line once: the shown ones are dropped by stamp, the cut one is
// read whole the second time. The stamps are nanoseconds apart, inside
// the second the reconnect asks from.
func TestReconnectRepeatsNothingAndLosesNothing(t *testing.T) {
	t.Parallel()

	const (
		a = "2026-10-21T12:00:01.000000100Z a\n"
		b = "2026-10-21T12:00:01.000000200Z b\n"
		c = "2026-10-21T12:00:01.000000300Z c, which is long\n"
		d = "2026-10-21T12:00:02.000000000Z d\n"
	)
	src := &replays{bodies: map[string][]string{"p": {a + b + c[:40] + "|cut", a + b + c + d}}}
	s := startSession(async.New(), src, "ns", defaultCrashLog(), pumpDirect, []string{"p"}, 100)
	t.Cleanup(s.stop)
	read := func() []logEvent {
		var out []logEvent
		for {
			select {
			case ev := <-s.events:
				out = append(out, ev)
				if ev.Done {
					return out
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("the stream never ended: %+v", out)
			}
		}
	}

	s.attach(attachReq{pod: "p", attempt: 1, opts: k8s.LogOptions{Tail: 100}})
	first := read()
	if first[len(first)-1].Err == nil {
		t.Fatalf("the cut stream ended cleanly: %+v", first)
	}
	resume := first[len(first)-2].TS
	s.attach(attachReq{pod: "p", attempt: 2, opts: k8s.LogOptions{Since: resume}, after: resume})
	second := read()

	got := append(lineTexts(first), lineTexts(second)...)
	if want := "a|b|c, which is long|d"; strings.Join(got, "|") != want {
		t.Fatalf("lines = %q, want %q", got, want)
	}
	if asked := src.request(1); !asked.Since.Equal(resume) || !asked.Timestamps {
		t.Errorf("the reconnect asked for %+v", asked)
	}
}
