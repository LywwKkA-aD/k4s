package status

import (
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

var debug = kubectl.New("debug", "-it", "-n", "demo", "web-1", "--image=busybox:1.36")

func about(level Level) Line {
	l, _ := Line{}.SetCommand(level, "no sh in demo/web-1", debug, t0)
	return l
}

func TestSetCommandShowsTheMessageAndArmsTheExpiry(t *testing.T) {
	t.Parallel()

	l, cmd := Line{}.SetCommand(Error, "no sh in demo/web-1", debug, t0)
	if cmd == nil {
		t.Fatal("SetCommand returned no expiry Cmd")
	}
	if got, ok := l.Current(t0); !ok || got != (Message{Level: Error, Text: "no sh in demo/web-1"}) {
		t.Fatalf("Current = %+v, %v", got, ok)
	}
}

// TestCommandGoesWithItsMessage: the command shows exactly while the
// message about it does.
func TestCommandGoesWithItsMessage(t *testing.T) {
	t.Parallel()

	replaced, _ := about(Error).Set(Error, "boom", at(time.Second))
	cases := []struct {
		name string
		line Line
		now  time.Time
		want bool
	}{
		{name: "while the message shows", line: about(Info), now: t0, want: true},
		{name: "gone at the ttl", line: about(Info), now: at(TTL)},
		{name: "a sticky error keeps it", line: about(Error), now: at(time.Hour), want: true},
		{name: "a key inside the ttl keeps it", line: about(Error).KeyPressed(), now: at(time.Second), want: true},
		{name: "a key after the ttl takes it", line: about(Error).KeyPressed(), now: at(time.Hour)},
		{name: "cleared", line: about(Error).Clear(), now: t0},
		{name: "replaced by a message without one", line: replaced, now: at(time.Second)},
		{name: "a message without one", line: set(Error, "boom"), now: t0},
		{name: "zero line", now: t0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.line.Command(tc.now)
			if ok != tc.want {
				t.Fatalf("Command showing = %v, want %v", ok, tc.want)
			}
			if ok && got.String() != debug.String() {
				t.Fatalf("Command = %q, want %q", got.String(), debug.String())
			}
		})
	}
}

// TestCommandAtKeySeesWhatTheKeyDismissed: the key that dismisses an error
// still finds the command the footer showed when it was pressed.
func TestCommandAtKeySeesWhatTheKeyDismissed(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		line Line
		now  time.Time
		want bool
	}{
		{name: "no key yet", line: about(Error), now: at(time.Hour), want: true},
		{name: "the key that dismisses the error", line: about(Error).KeyPressed(), now: at(time.Hour), want: true},
		{name: "a key after the one that dismissed it", line: about(Error).KeyPressed().KeyPressed(), now: at(time.Hour)},
		{name: "a second key inside the ttl", line: about(Error).KeyPressed().KeyPressed(), now: at(time.Second), want: true},
		{name: "a key after an info expired", line: about(Info).KeyPressed(), now: at(TTL)},
		{name: "a message without one", line: set(Error, "boom").KeyPressed(), now: at(time.Hour)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.line.CommandAtKey(tc.now)
			if ok != tc.want {
				t.Fatalf("CommandAtKey showing = %v, want %v", ok, tc.want)
			}
			if ok && got.String() != debug.String() {
				t.Fatalf("CommandAtKey = %q, want %q", got.String(), debug.String())
			}
		})
	}
}
