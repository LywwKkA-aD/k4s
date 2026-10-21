package logs

import (
	"strings"
	"testing"
	"time"
)

// The merger never reads a clock: every test hands it the laptop's time
// explicitly, and the node's time only arrives as line timestamps.

var t0 = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

func line(pod string, ts time.Time, text string) parsedEvent {
	return parsedEvent{pod: pod, ts: ts, text: text, kind: lineLog}
}

func ended(pod string) parsedEvent {
	return parsedEvent{pod: pod, kind: lineStreamDone}
}

func texts(evs []parsedEvent) string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		switch ev.kind {
		case lineStreamDone:
			out = append(out, ev.pod+":end")
		case lineStreamErr:
			out = append(out, ev.pod+":err")
		default:
			out = append(out, ev.text)
		}
	}
	return strings.Join(out, " ")
}

func TestMergerQuietPodDelaysHistoryAtMostIdle(t *testing.T) {
	t.Parallel()

	g := newMerger([]string{"a", "b"}, 100, t0)
	node := at(-time.Hour)
	arrived := at(10 * time.Millisecond)
	if out := g.add(line("a", node, "a1"), arrived); out != nil {
		t.Fatalf("history went out before it was done: %s", texts(out))
	}
	// Tick at every deadline the merger asks for, as the loop does.
	var got string
	var emitted time.Time
	for range 5 {
		due, ok := g.deadline()
		if !ok {
			break
		}
		if out := g.tick(due.Add(-time.Millisecond)); out != nil {
			t.Fatalf("history went out before its deadline: %s", texts(out))
		}
		if out := g.tick(due); out != nil {
			got, emitted = texts(out), due
			break
		}
	}
	if got != "a1" || emitted.Sub(arrived) > historyIdle {
		t.Fatalf("history %q went out %v after the first line, want a1 within %v", got, emitted.Sub(arrived), historyIdle)
	}
}

func TestMergerHistoryEndsOnTailCountWithoutWaiting(t *testing.T) {
	t.Parallel()

	g := newMerger([]string{"a", "b"}, 2, t0)
	node := at(-time.Hour)
	steps := []struct {
		ev   parsedEvent
		want string
	}{
		{ev: line("a", node.Add(1*time.Second), "a1")},
		{ev: line("b", node.Add(2*time.Second), "b1")},
		{ev: line("a", node.Add(3*time.Second), "a2")},
		{ev: line("b", node.Add(4*time.Second), "b2"), want: "a1 b1 a2 b2"},
	}
	for i, s := range steps {
		if got := texts(g.add(s.ev, t0)); got != s.want {
			t.Fatalf("step %d: emitted %q, want %q", i, got, s.want)
		}
	}
}

func TestMergerHardCapEndsABusyHistory(t *testing.T) {
	t.Parallel()

	g := newMerger([]string{"a"}, 1000, t0)
	var flushed time.Duration
	for d := time.Duration(0); d <= historyCap; d += 100 * time.Millisecond {
		out := g.add(line("a", at(d), "x"), at(d))
		if due, ok := g.deadline(); ok && !due.After(at(d+100*time.Millisecond)) {
			out = append(out, g.tick(due)...)
		}
		if len(out) > 0 {
			flushed = d
			break
		}
	}
	if flushed == 0 || flushed > historyCap {
		t.Fatalf("history flushed at %v, want by %v", flushed, historyCap)
	}
}

func TestMergerSortsWhateverTheNodeClockSays(t *testing.T) {
	t.Parallel()

	for _, skew := range []time.Duration{-10 * time.Minute, 0, 10 * time.Minute} {
		t.Run(skew.String(), func(t *testing.T) {
			t.Parallel()
			node := t0.Add(skew)
			g := newMerger([]string{"a", "b"}, 2, t0)
			// History arrives a pod at a time, as the streams race.
			g.add(line("a", node.Add(-4*time.Second), "a1"), at(1*time.Millisecond))
			g.add(line("a", node.Add(-2*time.Second), "a2"), at(2*time.Millisecond))
			g.add(line("b", node.Add(-3*time.Second), "b1"), at(3*time.Millisecond))
			got := texts(g.add(line("b", node.Add(-1*time.Second), "b2"), at(4*time.Millisecond)))
			if got != "a1 b1 a2 b2" {
				t.Fatalf("history = %q, want a1 b1 a2 b2", got)
			}

			// Live lines, and a slow pod's late history, sort within the hold.
			live := at(10 * time.Millisecond)
			g.add(line("a", node.Add(2*time.Second), "a3"), live)
			g.add(line("b", node.Add(1*time.Second), "b-late"), live.Add(5*time.Millisecond))
			if out := g.tick(live.Add(lateHold - time.Millisecond)); out != nil {
				t.Fatalf("held lines went out early: %s", texts(out))
			}
			if got := texts(g.tick(live.Add(lateHold))); got != "b-late a3" {
				t.Fatalf("held = %q, want b-late a3", got)
			}
		})
	}
}

func TestMergerEndsComeAfterTheirLines(t *testing.T) {
	t.Parallel()

	node := at(-time.Hour)
	g := newMerger([]string{"a", "b"}, 10, t0)
	g.add(line("b", node.Add(2*time.Second), "b1"), t0)
	g.add(ended("b"), t0)
	g.add(line("a", node.Add(1*time.Second), "a1"), t0)
	got := texts(g.add(ended("a"), t0))
	if got != "a1 b1 b:end a:end" {
		t.Fatalf("history = %q, want the lines in time order, then the ends", got)
	}

	g = newMerger([]string{"a"}, 1, t0)
	g.add(line("a", node, "a1"), t0)
	g.add(line("a", node.Add(time.Second), "a2"), t0)
	g.add(ended("a"), t0)
	if got := texts(g.tick(at(lateHold))); got != "a2 a:end" {
		t.Fatalf("live = %q, want the end after the last line", got)
	}
}

func TestMergerLineWithoutTimeStaysWithItsPod(t *testing.T) {
	t.Parallel()

	node := at(-time.Hour)
	g := newMerger([]string{"a", "b"}, 2, t0)
	g.add(line("a", node.Add(1*time.Second), "a1"), t0)
	g.add(line("b", node.Add(2*time.Second), "b1"), t0)
	g.add(line("a", time.Time{}, "a1-cont"), t0)
	got := texts(g.add(line("b", node.Add(3*time.Second), "b2"), t0))
	if got != "a1 a1-cont b1 b2" {
		t.Fatalf("history = %q, want the unstamped line right after a1", got)
	}
}

func TestMergerAFloodDoesNotWaitForTheHold(t *testing.T) {
	t.Parallel()

	g := newMerger([]string{"a"}, 1, t0)
	g.add(line("a", t0, "history"), t0)
	var released int
	for i := range maxHeld {
		out := g.add(line("a", t0.Add(time.Duration(maxHeld-i)*time.Millisecond), "x"), t0)
		if out != nil && i != maxHeld-1 {
			t.Fatalf("released %d lines after %d, before the heap was full", len(out), i+1)
		}
		released += len(out)
	}
	if released != maxHeld || len(g.held) != 0 {
		t.Fatalf("a full heap released %d lines and kept %d, want all %d out", released, len(g.held), maxHeld)
	}
}

func TestMergerHistoryIsBoundedWhileAPodIsSlow(t *testing.T) {
	t.Parallel()

	g := newMerger([]string{"busy", "slow"}, 2, t0)
	limit := 2*2 + maxHeld
	for i := range limit {
		out := g.add(line("busy", t0.Add(time.Duration(i)*time.Millisecond), "x"), t0)
		if i < limit-1 && out != nil {
			t.Fatalf("history flushed after %d lines, before its limit %d", i+1, limit)
		}
		if i == limit-1 && len(out) != limit {
			t.Fatalf("history at its limit released %d lines, want %d", len(out), limit)
		}
	}
}

func TestMergerDrainFlushesEverything(t *testing.T) {
	t.Parallel()

	node := at(-time.Hour)
	g := newMerger([]string{"a", "b"}, 100, t0)
	g.add(line("b", node.Add(2*time.Second), "b1"), t0)
	g.add(line("a", node.Add(1*time.Second), "a1"), t0)
	if got := texts(g.drain()); got != "a1 b1" {
		t.Fatalf("drain before the history was done = %q", got)
	}

	g = newMerger([]string{"a"}, 1, t0)
	g.add(line("a", node, "a1"), t0)
	g.add(line("a", node.Add(2*time.Second), "a3"), t0)
	g.add(line("a", node.Add(1*time.Second), "a2"), t0)
	if got := texts(g.drain()); got != "a2 a3" {
		t.Fatalf("drain of held lines = %q, want them sorted", got)
	}
	if _, ok := g.deadline(); ok {
		t.Error("a drained merger still has a deadline")
	}
}
