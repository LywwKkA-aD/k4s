package logs

import (
	"maps"
	"slices"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// phase is where a pod's stream stands.
type phase int

const (
	// phaseLive has a reader open.
	phaseLive phase = iota
	// phaseEnded has a reader that ended, why is not known yet.
	phaseEnded
	// phaseRetry was dropped and reconnects when its backoff is up.
	phaseRetry
	// phaseWaiting waits for the pod, or its container, to start.
	phaseWaiting
	// phaseDone is over for good.
	phaseDone
)

// podStream is the view's record of one pod's stream.
type podStream struct {
	phase phase
	// attempt numbers the reader now open, or the last one; events of
	// any other reader are stale.
	attempt uint64
	// container is what the pod streams: the view's container, or the
	// pod's own first one when it lacks the view's.
	container string
	// created is the pod's creation time: a pod that starts streaming
	// late reads its log from then.
	created time.Time
	// resume is the stamp of the last line shown: a reconnect reads from
	// there and drops what it has already shown.
	resume time.Time
	// restarts is the container's restart count when the reader opened.
	restarts int32
	// streamed is set once a reader has run for the pod.
	streamed bool
	err      error  // how the last reader ended
	endedAt  uint64 // tracker.ends when it ended
	// drops counts reconnects in a row, the backoff's step; a line read
	// resets it.
	drops int
	// note is the last notice said for the pod, so a wait is said once.
	note string
}

// stamp is the last line one reader sent in a batch.
type stamp struct {
	attempt uint64
	ts      time.Time
}

// tracker is every pod's stream, by name. It is a value: each change
// returns the next tracker over a fresh map, so no earlier copy of the
// view sees it.
type tracker struct {
	pods     map[string]podStream
	attempts uint64
	ends     uint64
}

func (t tracker) with(pod string, s podStream) tracker {
	pods := maps.Clone(t.pods)
	if pods == nil {
		pods = map[string]podStream{}
	}
	pods[pod] = s
	t.pods = pods
	return t
}

// open starts pod's next reader with opts, dropping lines stamped at or
// before after, and returns the request that opens it.
func (t tracker) open(pod string, s podStream, opts k8s.LogOptions, after time.Time) (tracker, attachReq) {
	t.attempts++
	s.phase, s.attempt, s.streamed, s.err, s.note = phaseLive, t.attempts, true, nil, ""
	opts.Container = s.container
	return t.with(pod, s), attachReq{pod: pod, attempt: t.attempts, opts: opts, after: after}
}

// saw records the last line each current reader sent. A line proves the
// connection, so the backoff starts over.
func (t tracker) saw(last map[string]stamp) tracker {
	if len(last) == 0 {
		return t
	}
	pods := maps.Clone(t.pods)
	for pod, st := range last {
		s, ok := pods[pod]
		if !ok || s.attempt != st.attempt {
			continue
		}
		if st.ts.After(s.resume) {
			s.resume = st.ts
		}
		s.drops = 0
		pods[pod] = s
	}
	t.pods = pods
	return t
}

// ended records that pod's reader attempt ended with err (nil for the end
// of the log). ok is false for a reader the view has moved past.
func (t tracker) ended(pod string, attempt uint64, err error) (tracker, bool) {
	s, known := t.pods[pod]
	if !known || s.attempt != attempt || s.phase != phaseLive {
		return t, false
	}
	t.ends++
	s.phase, s.err, s.endedAt = phaseEnded, err, t.ends
	return t.with(pod, s), true
}

// recheck marks a waiting pod as one the next check looks at again.
func (t tracker) recheck(pod string) tracker {
	s, ok := t.pods[pod]
	if !ok || s.phase != phaseWaiting {
		return t
	}
	t.ends++
	s.phase, s.err, s.endedAt = phaseEnded, nil, t.ends
	return t.with(pod, s)
}

// unexplained reports a stream that ended after upTo, which the check
// that covered ends up to upTo could not explain.
func (t tracker) unexplained(upTo uint64) bool {
	for _, s := range t.pods {
		if s.phase == phaseEnded && s.endedAt > upTo {
			return true
		}
	}
	return false
}

// endedBy lists, by name, the pods whose stream ended by upTo.
func (t tracker) endedBy(upTo uint64) []string {
	var out []string
	for pod, s := range t.pods {
		if s.phase == phaseEnded && s.endedAt <= upTo {
			out = append(out, pod)
		}
	}
	slices.Sort(out)
	return out
}

// inPhase lists, by name, the pods in phase p.
func (t tracker) inPhase(p phase) []string {
	var out []string
	for pod, s := range t.pods {
		if s.phase == p {
			out = append(out, pod)
		}
	}
	slices.Sort(out)
	return out
}

// active lists, by name, the pods streamed or about to be: all but those
// done.
func (t tracker) active() []string {
	out := make([]string, 0, len(t.pods))
	for pod, s := range t.pods {
		if s.phase != phaseDone {
			out = append(out, pod)
		}
	}
	slices.Sort(out)
	return out
}
