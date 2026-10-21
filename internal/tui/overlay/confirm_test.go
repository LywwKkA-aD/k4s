package overlay_test

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestRestoreConfirmKeys(t *testing.T) {
	t.Parallel()

	revive := overlay.RestoreDecisionMsg{Revive: true}
	skip := overlay.RestoreDecisionMsg{}
	cases := []struct {
		key  string
		want *overlay.RestoreDecisionMsg // nil: the key is ignored
	}{
		{key: "y"},
		{key: "Y"},
		{key: "enter", want: &revive},
		{key: "n", want: &skip},
		{key: "N", want: &skip},
		{key: "q", want: &skip},
		{key: "esc", want: &skip},
		{key: "x"},
		{key: ":"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			o := overlay.NewRestore(2, clk.Now)
			clk.Advance(overlay.Grace)
			o, out := feed(t, o, tuitest.Press(tc.key))
			if tc.want == nil {
				requireOpen(t, o)
				return
			}
			if o != nil || len(out) != 1 || !tuitest.Contains(out, *tc.want) {
				t.Fatalf("open=%v, messages %s, want %+v", o != nil, tuitest.Describe(out), *tc.want)
			}
		})
	}
}

// TestConfirmGracePeriod: keys typed ahead, before the user can have read
// the question, must not answer it.
func TestConfirmGracePeriod(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		after  time.Duration
		answer bool
	}{
		{name: "at once", after: 0},
		{name: "just inside", after: overlay.Grace - time.Millisecond},
		{name: "at the grace", after: overlay.Grace, answer: true},
		{name: "later", after: time.Minute, answer: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := newClock()
			o := overlay.NewRestore(1, clk.Now)
			clk.Advance(tc.after)
			next, cmd := o.Update(tuitest.Enter())
			if !tc.answer {
				if next == nil || cmd != nil {
					t.Fatalf("a key %v after opening answered", tc.after)
				}
				return
			}
			if next != nil || cmd == nil {
				t.Fatalf("a key %v after opening was dropped", tc.after)
			}
		})
	}
}

func TestRestoreConfirmView(t *testing.T) {
	t.Parallel()

	now := newClock().Now
	cases := []struct {
		count int
		want  string
	}{
		{count: 1, want: "Found 1 saved forward from a previous session."},
		{count: 3, want: "Found 3 saved forwards from a previous session."},
	}
	for _, tc := range cases {
		requireShows(t, overlay.NewRestore(tc.count, now), "restore port-forwards", tc.want, "Enter revive all · n / Esc skip")
	}
	o := overlay.NewRestore(3, now)
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestRestoreListsTheClusters(t *testing.T) {
	t.Parallel()

	o := overlay.NewRestore(3, newClock().Now, "default · 127.0.0.1:6443", "default · unknown cluster, confirm")
	requireShows(t, o,
		"Found 3 saved forwards from a previous session, for:",
		"\n  default · 127.0.0.1:6443\n",
		"\n  default · unknown cluster, confirm\n",
		"Enter revive all · n / Esc skip",
	)
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestConfirmIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	clk := newClock()
	o := overlay.NewRestore(1, clk.Now)
	clk.Advance(time.Hour)
	o, _ = feed(t, o, tuitest.Paste("y"), tuitest.Resize(80, 24))
	requireShows(t, o, "restore port-forwards")
}

func TestConfirmWithoutAnAnswerMessage(t *testing.T) {
	t.Parallel()

	clk := newClock()
	o := overlay.NewConfirm(overlay.Confirm{Title: "really?", Question: "Drop it?", Yes: overlay.RestoreDecisionMsg{Revive: true}}, clk.Now)
	requireShows(t, o, "really?", "Drop it?")
	clk.Advance(overlay.Grace)
	next, cmd := o.Update(tuitest.Press("n"))
	if next != nil || cmd != nil {
		t.Fatalf("no closed with open=%v cmd=%v", next != nil, cmd != nil)
	}
}
