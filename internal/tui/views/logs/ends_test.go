package logs

import (
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// stamped is the node's time sec seconds and ns nanoseconds past created.
func stamped(sec, ns int64) time.Time {
	return created.Add(time.Duration(sec)*time.Second + time.Duration(ns))
}

func onePod(t *testing.T, pod *corev1.Pod) *rig {
	t.Helper()
	return newRig(t, webObjects(pod), "", "", pod.Name)
}

func TestADroppedStreamReconnectsFromItsLastLine(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.feed(r.line("web-h1-a", stamped(5, 100), "one"), r.line("web-h1-a", stamped(5, 200), "two"), r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(explainDebounce)
	if strings.Contains(r.screen(), "◇") {
		t.Fatalf("the drop is told before the reconnect:\n%s", r.screen())
	}
	r.advance(time.Second)
	r.shows("[web-h1-a] ◇ stream dropped · reconnected")
	opts := r.src.opened(t, "web-h1-a", 2)
	if want := (k8s.LogOptions{Since: stamped(5, 200), Timestamps: true}); opts != want {
		t.Errorf("the reconnect asked for %+v, want %+v", opts, want)
	}
	if got := r.m.streams.pods["web-h1-a"].resume; !got.Equal(stamped(5, 200)) {
		t.Errorf("resume point = %v", got)
	}
	if got := r.calls(); got["get pods"] != 1 || len(got) != 1 {
		t.Errorf("explaining the drop made %v, want one GET", got)
	}
}

func TestDropsBackOff(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	retries := make([]time.Duration, 0, 6)
	for range 6 {
		before := len(r.clock.Delays())
		r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
		r.advance(explainDebounce)
		delays := r.clock.Delays()[before:]
		retries = append(retries, delays[len(delays)-1])
		r.advance(delays[len(delays)-1])
	}
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 30 * time.Second, 30 * time.Second}
	if !slices.Equal(retries, want) {
		t.Fatalf("retry delays = %v, want %v", retries, want)
	}
	// A line proves the connection: the next drop starts over.
	r.feed(r.line("web-h1-a", stamped(9, 0), "back"), r.end("web-h1-a", io.ErrUnexpectedEOF))
	before := len(r.clock.Delays())
	r.advance(explainDebounce)
	if got := r.clock.Delays()[before:]; got[len(got)-1] != time.Second {
		t.Errorf("after a line the retry waits %v", got)
	}
}

func TestARestartReattachesToTheNewInstance(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.feed(r.line("web-h1-a", stamped(1, 0), "allocating"), r.end("web-h1-a", nil))
	r.mutate("web-h1-a", func(p *corev1.Pod) {
		cs := &p.Status.ContainerStatuses[0]
		cs.RestartCount = 1
		cs.LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}
	})
	r.advance(explainDebounce)
	r.shows("[web-h1-a] ◇ container restarted (OOMKilled, exit 137) · reattached")
	if opts := r.src.opened(t, "web-h1-a", 2); opts != (k8s.LogOptions{Tail: 100, Timestamps: true}) {
		t.Errorf("the new instance was asked for %+v", opts)
	}
}

func TestAnExitWaitsForTheRestart(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.mutate("web-h1-a", func(p *corev1.Pod) {
		cs := &p.Status.ContainerStatuses[0]
		cs.State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}
		cs.LastTerminationState = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}
	})
	r.feed(r.end("web-h1-a", nil))
	r.advance(explainDebounce)
	r.shows("[web-h1-a] ◇ container exited (Error, exit 1) · waiting for a restart")
	r.advance(time.Second + explainDebounce)
	if strings.Count(r.screen(), "waiting for a restart") != 1 || r.src.count("web-h1-a") != 1 {
		t.Fatalf("the wait repeated or reattached early:\n%s", r.screen())
	}
	r.mutate("web-h1-a", func(p *corev1.Pod) {
		cs := &p.Status.ContainerStatuses[0]
		cs.RestartCount = 1
		cs.State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}}
	})
	r.advance(2*time.Second + explainDebounce)
	r.shows("[web-h1-a] ◇ container restarted (Error, exit 1) · reattached")
	r.src.opened(t, "web-h1-a", 2)
}

func TestEndsThatLastSayWhy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		change func(*rig)
		err    error
		want   string
	}{
		{name: "deleted", change: func(r *rig) { r.remove("web-h1-a") }, want: "[web-h1-a] ○ ended: pod deleted"},
		{
			name: "being deleted",
			change: func(r *rig) {
				r.mutate("web-h1-a", func(p *corev1.Pod) { p.DeletionTimestamp = &created })
			},
			want: "[web-h1-a] ○ ended: pod deleted",
		},
		{
			name: "completed",
			change: func(r *rig) {
				r.mutate("web-h1-a", func(p *corev1.Pod) {
					p.Status.Phase = corev1.PodSucceeded
					p.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed"}}
				})
			},
			want: "[web-h1-a] ○ ended: pod completed (exit 0)",
		},
		{
			name: "evicted",
			change: func(r *rig) {
				r.mutate("web-h1-a", func(p *corev1.Pod) { p.Status.Phase, p.Status.Reason = corev1.PodFailed, "Evicted" })
			},
			want: "[web-h1-a] ○ ended: pod failed · Evicted",
		},
		{
			name: "failed",
			change: func(r *rig) {
				r.mutate("web-h1-a", func(p *corev1.Pod) {
					p.Status.Phase = corev1.PodFailed
					p.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error", ExitCode: 1}}
				})
			},
			want: "[web-h1-a] ○ ended: pod failed · Error (exit 1)",
		},
		{
			name: "refused", change: func(*rig) {}, err: apierrors.NewUnauthorized("token expired"),
			want: "[web-h1-a] ○ ended: token expired",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := onePod(t, webPod("web-h1-a", "h1", 0))
			tc.change(r)
			r.feed(r.end("web-h1-a", tc.err))
			r.advance(explainDebounce)
			r.shows(tc.want)
			r.advance(time.Minute)
			if r.src.count("web-h1-a") != 1 || r.clock.Pending() != 0 {
				t.Errorf("an ended stream reconnects: %d streams, %d timers", r.src.count("web-h1-a"), r.clock.Pending())
			}
			if got := r.m.Command().String(); got != "kubectl logs -f web-h1-a --tail=100 -n shop" {
				t.Errorf("footer = %q", got)
			}
		})
	}
}

func TestAPodThatIsNotStartedWaits(t *testing.T) {
	t.Parallel()

	r := onePod(t, pending(webPod("web-h1-a", "h1", 30)))
	refused := apierrors.NewBadRequest(`container "app" in pod "web-h1-a" is waiting to start: ContainerCreating`)
	r.feed(r.end("web-h1-a", refused))
	r.advance(explainDebounce)
	r.shows("[web-h1-a] △ waiting: ContainerCreating")
	r.advance(time.Second + explainDebounce)
	if strings.Count(r.screen(), "△ waiting") != 1 {
		t.Errorf("the wait repeated:\n%s", r.screen())
	}
	r.mutate("web-h1-a", func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
		p.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}}
	})
	r.advance(2*time.Second + explainDebounce)
	if opts := r.src.opened(t, "web-h1-a", 2); opts.Since.IsZero() && opts.Tail != 100 {
		t.Errorf("the started pod was asked for %+v", opts)
	}
	if strings.Contains(r.screen(), "◇ stream dropped") {
		t.Errorf("a start is told as a drop:\n%s", r.screen())
	}
}

func TestEndsInOneWindowShareOneList(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(explainDebounce / 2)
	r.feed(r.end("web-h1-b", io.ErrUnexpectedEOF))
	r.advance(explainDebounce)
	if got := r.calls(); got["list pods"] != 1 || len(got) != 1 {
		t.Errorf("two ends a moment apart made %v, want one pod list", got)
	}
	r.advance(time.Second)
	r.shows("[web-h1-a] ◇ stream dropped · reconnected", "[web-h1-b] ◇ stream dropped · reconnected")
}

func TestThePreviousInstanceIsNeverReconnected(t *testing.T) {
	t.Parallel()

	r := onePod(t, webPod("web-h1-a", "h1", 0))
	r.send(tuitest.Press("p"))
	if !r.m.previous {
		t.Fatal("p did not open the previous instance")
	}
	r.send(logEventsBatch{tok: r.m.sess.tok, events: []logEvent{{Pod: "web-h1-a", Line: "last words"}, {Pod: "web-h1-a", Done: true}}})
	r.advance(time.Minute)
	r.shows("last words", "[web-h1-a] ○ end of the previous instance's log")
	if got := r.calls(); got["get pods"] != 0 || r.src.count("web-h1-a") != 2 {
		t.Errorf("the previous log was explained or reopened: %v, %d streams", got, r.src.count("web-h1-a"))
	}
	if strings.Contains(r.m.Command().String(), "--timestamps") {
		t.Errorf("the footer shows k4s's own timestamps: %s", r.m.Command())
	}
}
