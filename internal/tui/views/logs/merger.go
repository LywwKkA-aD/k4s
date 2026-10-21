package logs

import (
	"container/heap"
	"slices"
	"time"
)

// Multi-pod streams are merged in two phases, without ever comparing the
// laptop's clock with a node's:
//
//  1. History. Each pod's tail is held until the pod has sent its tail
//     lines, gone quiet for historyIdle (the timer restarts on each of its
//     events) or ended. Once every pod is done, or historyCap has passed,
//     the history goes out sorted by line time. Without this the user sees
//     all of pod A's history, then all of pod B's, as the streams race.
//  2. Live. A line after that waits up to lateHold in a heap ordered by
//     line time, so the late history of a slow pod still lands among the
//     lines of its neighbours.
//
// Line times are only compared with line times. The laptop's clock times
// the waiting alone, and a node clock minutes off changes nothing.
//
// A flood lets the heap go at maxHeld events instead of waiting: memory
// stays bounded, and a view that cannot keep up slows the readers down.
const (
	historyIdle = 300 * time.Millisecond
	historyCap  = 5 * time.Second
	lateHold    = 250 * time.Millisecond
	maxHeld     = drainBatchCap
)

// parsedEvent is one item from a pod's reader: a line with the time the
// API server stamped on it, or the end of the pod's stream.
type parsedEvent struct {
	pod  string
	text string
	ts   time.Time
	kind lineKind
	seq  uint64 // arrival order, which breaks ties in time
}

// podProgress is how far a pod's history has come.
type podProgress struct {
	lines  int64
	last   time.Time // laptop time of its last event, or the start
	lastTS time.Time // stamp of its last line, lent to an unstamped one
	done   bool
}

// merger is the merge's state. It reads no clock: each call gets the
// laptop's time, so the timing is tested without waiting. One goroutine
// owns it, and its methods update it in place.
type merger struct {
	tail int64
	// limit is the most history held: every pod's tail, and maxHeld for
	// the lines a done pod sends while another one is slow.
	limit   int
	start   time.Time
	pods    map[string]podProgress
	history []parsedEvent
	ends    []parsedEvent // streams that ended during the history
	flushed bool
	held    eventHeap
	heldAt  time.Time // arrival of the first event held now
	seq     uint64
}

func newMerger(pods []string, tail int64, now time.Time) *merger {
	progress := make(map[string]podProgress, len(pods))
	for _, p := range pods {
		progress[p] = podProgress{last: now}
	}
	limit := int(max(tail, 0))*len(pods) + maxHeld
	return &merger{tail: tail, limit: limit, start: now, pods: progress}
}

// add takes one event that arrived at now and returns what may go out.
func (g *merger) add(ev parsedEvent, now time.Time) []parsedEvent {
	p := g.pods[ev.pod]
	if ev.kind == lineLog && !ev.ts.IsZero() {
		p.lastTS = ev.ts
	} else {
		ev.ts = p.lastTS
	}
	ev.seq = g.seq
	g.seq++
	if g.flushed {
		g.pods[ev.pod] = p
		if len(g.held) == 0 {
			g.heldAt = now
		}
		heap.Push(&g.held, ev)
		return g.release(now)
	}
	p.last = now
	if ev.kind == lineLog {
		g.history = append(g.history, ev)
		p.lines++
		p.done = p.done || p.lines >= g.tail
	} else {
		g.ends = append(g.ends, ev)
		p.done = true
	}
	g.pods[ev.pod] = p
	return g.flush(now)
}

// tick returns what may go out at now.
func (g *merger) tick(now time.Time) []parsedEvent {
	if !g.flushed {
		return g.flush(now)
	}
	return g.release(now)
}

// deadline is the next time tick has something to do.
func (g *merger) deadline() (time.Time, bool) {
	if g.flushed {
		if len(g.held) == 0 {
			return time.Time{}, false
		}
		return g.heldAt.Add(lateHold), true
	}
	due := g.start.Add(historyCap)
	for _, p := range g.pods {
		if !p.done {
			due = earliest(due, p.last.Add(historyIdle))
		}
	}
	return due, true
}

// drain returns everything still held, in order: the streams have ended.
func (g *merger) drain() []parsedEvent {
	out := g.flush(g.start.Add(historyCap))
	g.heldAt = time.Time{}
	for len(g.held) > 0 {
		out = append(out, heap.Pop(&g.held).(parsedEvent))
	}
	return out
}

// flush ends the history once every pod is done, counting a pod quiet for
// historyIdle as done, once historyCap has passed, or once it holds limit
// lines.
func (g *merger) flush(now time.Time) []parsedEvent {
	if g.flushed {
		return nil
	}
	pending := false
	for name, p := range g.pods {
		if !p.done && now.Sub(p.last) >= historyIdle {
			p.done = true
			g.pods[name] = p
		}
		pending = pending || !p.done
	}
	if pending && now.Sub(g.start) < historyCap && len(g.history) < g.limit {
		return nil
	}
	g.flushed = true
	out := slices.Clone(g.history)
	slices.SortStableFunc(out, func(a, b parsedEvent) int { return a.ts.Compare(b.ts) })
	out = append(out, g.ends...)
	g.history, g.ends = nil, nil
	return out
}

// release lets everything held go, in time order, once the first of it
// has waited lateHold or the heap is full.
func (g *merger) release(now time.Time) []parsedEvent {
	if len(g.held) == 0 || (now.Sub(g.heldAt) < lateHold && len(g.held) < maxHeld) {
		return nil
	}
	out := make([]parsedEvent, 0, len(g.held))
	for len(g.held) > 0 {
		out = append(out, heap.Pop(&g.held).(parsedEvent))
	}
	return out
}

func earliest(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// eventHeap orders held events by line time, then by arrival.
type eventHeap []parsedEvent

func (h eventHeap) Len() int { return len(h) }

func (h eventHeap) Less(i, j int) bool {
	if c := h[i].ts.Compare(h[j].ts); c != 0 {
		return c < 0
	}
	return h[i].seq < h[j].seq
}

func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

// Push implements heap.Interface.
func (h *eventHeap) Push(x any) { *h = append(*h, x.(parsedEvent)) }

// Pop implements heap.Interface.
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	ev := old[n-1]
	*h = old[:n-1]
	return ev
}
