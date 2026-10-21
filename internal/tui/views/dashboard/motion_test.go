package dashboard_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// mountMoving is mount without letting the motion come to rest.
func mountMoving(t *testing.T, f *feed, w, h int, opts ...dashboard.Option) dashboard.Model {
	t.Helper()
	m := dashboard.New(tuitest.NewFakeClient(t), nil, append(f.options(), opts...)...)
	m, _ = tuitest.Mount(t, m, w, h)
	return m
}

// TestFirstSnapshotSpringsUp: on the first snapshot the bars and counts
// spring up from zero, with the wordmark's shimmer in the wide layout, in
// at most 8 frames; then the clock stops.
func TestFirstSnapshotSpringsUp(t *testing.T) {
	t.Parallel()

	for _, size := range [][2]int{{wideW, wideH}, {120, 34}, {80, 18}} {
		f := newFeed(busyK3s)
		m := mountMoving(t, f, size[0], size[1])
		if strings.Contains(tuitest.Screen(m), "185") {
			t.Fatalf("%v: the pods total did not start from zero", size)
		}
		m, fired := f.settle(t, m)
		if n := framesIn(fired); n < 2 || n > 8 {
			t.Fatalf("%v: first snapshot ran %d frames, want 2 to 8", size, n)
		}
		if got, want := tuitest.Screen(m), tuitest.Screen(mount(t, newFeed(busyK3s), size[0], size[1])); got != want {
			t.Fatalf("%v: the spring did not come to rest where the data is:\n%s\n---\n%s", size, got, want)
		}
	}
}

// TestFocusSweep: a panel newly focused sweeps a gradient along its
// border in at most 6 frames. The sweep only recolours: stripped, a frame
// mid-sweep equals one at rest. Moving the row cursor sweeps nothing.
func TestFocusSweep(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m, _ := press(t, mount(t, f, wideW, wideH), "3")
	mid := tuitest.Frame(m)
	m, fired := f.settle(t, m)
	if n := framesIn(fired); n < 1 || n > 6 {
		t.Fatalf("focus change ran %d frames, want 1 to 6", n)
	}
	rest := tuitest.Frame(m)
	if tuitest.Strip(mid) != tuitest.Strip(rest) {
		t.Fatal("a sweep changed the text, not only its colour")
	}
	if _, _ = press(t, m, "down", "down"); f.frames.Pending() != 0 {
		t.Fatal("a row move armed a frame")
	}
}

// TestVerdictFlipSweeps: attention turning into all clear sweeps the
// verdict word, at most 6 frames.
func TestVerdictFlipSweeps(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	f.set(allClear)
	m = advance(t, m, f, 5*time.Second)
	_, fired := f.settle(t, m)
	if n := framesIn(fired); n < 1 || n > 6 {
		t.Fatalf("a verdict flip ran %d frames, want 1 to 6", n)
	}
}

// TestIdleIsStill is the idle guard (§10.3, critique): with no input and
// no cluster change, a minute brings only the poll's ticks and replies to
// Update, and no frame is armed. Between two metrics samples (15s) the
// view repaints byte for byte; a sample only adds a point to the ≈
// history.
func TestIdleIsStill(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	updates := 0
	views := make([]string, 0, 12)
	for range 12 {
		ticks := f.clock.Advance(tickStep)
		var out []tea.Msg
		m, out = tuitest.Drive(t, m, ticks...)
		updates += len(ticks) + len(out)
		for _, msg := range out {
			if name := fmt.Sprintf("%T", msg); !strings.Contains(name, "async.Result") {
				t.Fatalf("idle poll delivered %s", name)
			}
		}
		views = append(views, tuitest.Frame(m))
	}
	if updates != 24 {
		t.Fatalf("a still minute took %d updates, want 12 ticks and 12 replies", updates)
	}
	if f.frames.Pending() != 0 {
		t.Fatalf("%d frames armed at idle", f.frames.Pending())
	}
	// Ticks at 5s and 10s are fast-tier only: nothing new to draw.
	if views[0] != views[1] {
		t.Fatal("an idle poll without a metrics sample changed the view")
	}
}

// TestNoMotionWithoutColour: on the ASCII and NoTTY profiles nothing
// moves: the data shows at once and no frame or spinner is armed.
func TestNoMotionWithoutColour(t *testing.T) {
	t.Parallel()

	for _, p := range []colorprofile.Profile{colorprofile.ASCII, colorprofile.NoTTY} {
		f := newFeed(busyK3s)
		m := mountMoving(t, f, wideW, wideH, dashboard.WithColorProfile(p))
		if f.frames.Pending() != 0 {
			t.Fatalf("%v: %d frames armed", p, f.frames.Pending())
		}
		if !strings.Contains(tuitest.Screen(m), "185") {
			t.Fatalf("%v: the data did not show at once", p)
		}
		if _, _ = press(t, m, "3", "r"); f.frames.Pending() != 0 {
			t.Fatalf("%v: focus or r armed %d frames", p, f.frames.Pending())
		}
	}
}

// TestProfileMessageStopsMotion: the program's report of a colourless
// terminal stops the motion in progress where it would end.
func TestProfileMessageStopsMotion(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mountMoving(t, f, wideW, wideH)
	m, _ = tuitest.Drive(t, m, tea.ColorProfileMsg{Profile: colorprofile.ASCII})
	if !strings.Contains(tuitest.Screen(m), "185") {
		t.Fatal("the spring did not snap to its end")
	}
	if _, fired := f.settle(t, m); framesIn(fired) > 1 {
		t.Fatalf("%d frames after the profile said ASCII, want at most the one in flight", framesIn(fired))
	}
}

// TestBlurStopsMotion: a terminal in the background moves nothing until
// it has focus again.
func TestBlurStopsMotion(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mountMoving(t, f, wideW, wideH)
	m, _ = tuitest.Drive(t, m, tuitest.Blur())
	if !strings.Contains(tuitest.Screen(m), "185") {
		t.Fatal("blur did not snap the spring to its end")
	}
	m, fired := f.settle(t, m)
	if framesIn(fired) > 1 {
		t.Fatalf("%d frames after blur", framesIn(fired))
	}
	m, _ = press(t, m, "3")
	if f.frames.Pending() != 0 {
		t.Fatal("a focus change while blurred armed a frame")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Focus())
	if _, _ = press(t, m, "4"); f.frames.Pending() == 0 {
		t.Fatal("focus regained, a focus change armed no frame")
	}
}

// TestHeartbeatSteps: the heartbeat moves one step per snapshot that
// changed, and only then.
func TestHeartbeatSteps(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	beat := func() string {
		line := strings.TrimRight(bannerLine(t, tuitest.Screen(m), 0), " ")
		return line[strings.LastIndex(line, " ")+1:]
	}
	if got := beat(); got != "⣽" {
		t.Fatalf("heartbeat after the first snapshot = %q", got)
	}
	m = advance(t, m, f, 10*time.Second)
	if got := beat(); got != "⣽" {
		t.Fatalf("unchanged polls moved the heartbeat to %q", got)
	}
	f.set(func() dash.Snapshot {
		s := busyK3s()
		s.Pods.Total = 186
		return s
	})
	m = advance(t, m, f, 5*time.Second)
	if got := beat(); got != "⣻" {
		t.Fatalf("a changed poll left the heartbeat at %q", got)
	}
	if f.frames.Pending() != 0 {
		t.Fatal("a heartbeat step armed a frame")
	}
}

// TestSpinnerOnSlowRefresh: r still running after 150 ms swaps the
// heartbeat for the MiniDot spinner until the answer comes.
func TestSpinnerOnSlowRefresh(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m := mount(t, f, wideW, wideH)
	m, reply := tuitest.Step(t, m, tuitest.Press("r"))
	due, ok := f.frames.Fire()
	if !ok {
		t.Fatal("r armed no spinner delay")
	}
	m, _ = tuitest.Drive(t, m, due)
	if line := bannerLine(t, tuitest.Screen(m), 0); !strings.Contains(line, "refreshing… ⠋") {
		t.Fatalf("a slow refresh shows %q, want the spinner", line)
	}
	m, _ = tuitest.Drive(t, m, reply...)
	m, _ = f.settle(t, m)
	if line := bannerLine(t, tuitest.Screen(m), 0); !strings.HasSuffix(line, "watch 5s ⣽ ") {
		t.Fatalf("after the answer: %q, want the heartbeat back", line)
	}
}

// TestNewProblemRowsAreTagged: a problem row that appears after the first
// snapshot carries a muted "new" until the first poll 30s later.
func TestNewProblemRowsAreTagged(t *testing.T) {
	t.Parallel()

	without := func() dash.Snapshot {
		s := busyK3s()
		s.Pods.Problems = s.Pods.Problems[:5]
		return s
	}
	f := newFeed(without)
	m := mount(t, f, wideW, wideH)
	if strings.Contains(tuitest.Screen(m), " new ") {
		t.Fatal("the first snapshot tagged rows as new")
	}
	f.set(busyK3s)
	m = advance(t, m, f, 5*time.Second)
	row := func() string {
		for _, l := range strings.Split(tuitest.Screen(m), "\n") {
			if strings.Contains(l, "runner-build-8812-k7x2p ") && strings.Contains(l, "<none>") {
				return l
			}
		}
		return ""
	}
	if !strings.Contains(row(), "<none>  new") {
		t.Fatalf("the new row is not tagged: %q", row())
	}
	m = advance(t, m, f, 25*time.Second)
	if !strings.Contains(row(), "  new") {
		t.Fatalf("the tag went before 30s: %q", row())
	}
	m = advance(t, m, f, 5*time.Second)
	if strings.Contains(row(), "  new") {
		t.Fatalf("the tag stayed past 30s: %q", row())
	}
}
