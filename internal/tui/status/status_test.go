package status

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return t0.Add(d) }

func set(level Level, text string) Line {
	l, _ := Line{}.Set(level, text, t0)
	return l
}

func TestZeroLineShowsNothing(t *testing.T) {
	t.Parallel()

	if msg, ok := (Line{}).Current(t0); ok {
		t.Fatalf("zero Line shows %+v", msg)
	}
}

func TestSetShowsTheMessageAndArmsTheExpiry(t *testing.T) {
	t.Parallel()

	l, cmd := Line{}.Set(Warn, "deployment web has 0 replicas", t0)
	if cmd == nil {
		t.Fatal("Set returned no expiry Cmd")
	}
	got, ok := l.Current(t0)
	want := Message{Level: Warn, Text: "deployment web has 0 replicas"}
	if !ok || got != want {
		t.Fatalf("Current = %+v, %v; want %+v", got, ok, want)
	}
}

func TestMessagesLiveForTheTTL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		level   Level
		keyAt   time.Duration // < 0: no key pressed
		lookAt  time.Duration
		showing bool
	}{
		{name: "info inside the ttl", level: Info, keyAt: -1, lookAt: TTL - time.Millisecond, showing: true},
		{name: "info at the ttl", level: Info, keyAt: -1, lookAt: TTL},
		{name: "ok after the ttl", level: OK, keyAt: -1, lookAt: 2 * TTL},
		{name: "warn survives a key inside the ttl", level: Warn, keyAt: time.Second, lookAt: 2 * time.Second, showing: true},
		{name: "warn ignores keys after the ttl", level: Warn, keyAt: -1, lookAt: TTL},
		{name: "error outlives the ttl without a key", level: Error, keyAt: -1, lookAt: time.Hour, showing: true},
		{name: "error stays to the ttl after an early key", level: Error, keyAt: time.Second, lookAt: TTL - time.Millisecond, showing: true},
		{name: "error goes at the ttl after an early key", level: Error, keyAt: time.Second, lookAt: TTL},
		{name: "error goes on the first key after the ttl", level: Error, keyAt: time.Minute, lookAt: time.Minute},
		{name: "alert inside the ttl", level: Alert, keyAt: -1, lookAt: TTL - time.Millisecond, showing: true},
		{name: "alert goes at the ttl without a key", level: Alert, keyAt: -1, lookAt: TTL},
		{name: "alert survives a key inside the ttl", level: Alert, keyAt: time.Second, lookAt: 2 * time.Second, showing: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			l := set(tc.level, "boom")
			if tc.keyAt >= 0 {
				l = l.KeyPressed()
			}
			if _, ok := l.Current(at(tc.lookAt)); ok != tc.showing {
				t.Fatalf("showing at %v = %v, want %v", tc.lookAt, ok, tc.showing)
			}
		})
	}
}

func TestKeyPressedLeavesTheOriginalAlone(t *testing.T) {
	t.Parallel()

	before := set(Error, "boom")
	_ = before.KeyPressed()
	if _, ok := before.Current(at(time.Hour)); !ok {
		t.Fatal("KeyPressed changed the Line it was called on")
	}
}

func TestSetReplacesAndForgetsTheKey(t *testing.T) {
	t.Parallel()

	l := set(Error, "first").KeyPressed()
	l, _ = l.Set(Error, "second", at(time.Minute))
	got, ok := l.Current(at(time.Hour))
	if !ok || got.Text != "second" {
		t.Fatalf("Current = %+v, %v; want the second error still showing", got, ok)
	}
}

func TestClear(t *testing.T) {
	t.Parallel()

	l := set(Error, "boom").Clear()
	if msg, ok := l.Current(t0); ok {
		t.Fatalf("cleared Line shows %+v", msg)
	}
}

func TestExpire(t *testing.T) {
	t.Parallel()

	first, _ := Line{}.Set(Info, "first", t0)
	firstExpiry := ExpireMsg{seq: first.seq}
	second, _ := first.Set(Info, "second", at(4*time.Second))

	cases := []struct {
		name string
		line Line
		msg  ExpireMsg
		now  time.Time
		want string // "" means nothing showing afterwards
	}{
		{name: "own expiry clears", line: first, msg: firstExpiry, now: at(TTL)},
		{name: "stale expiry is ignored", line: second, msg: firstExpiry, now: at(TTL), want: "second"},
		{name: "sticky error survives its expiry", line: set(Error, "boom"), msg: ExpireMsg{seq: 1}, now: at(TTL), want: "boom"},
		{name: "early expiry keeps the message", line: first, msg: firstExpiry, now: at(time.Second), want: "first"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := tc.line.Expire(tc.msg, tc.now).Current(tc.now)
			if tc.want == "" {
				if ok {
					t.Fatalf("still showing %+v", got)
				}
				return
			}
			if !ok || got.Text != tc.want {
				t.Fatalf("Current = %+v, %v; want %q", got, ok, tc.want)
			}
		})
	}
}

func TestExpireAfterClearDoesNotRevive(t *testing.T) {
	t.Parallel()

	l, _ := Line{}.Set(Info, "first", t0)
	expiry := ExpireMsg{seq: l.seq}
	l = l.Clear()
	l, _ = l.Set(Info, "second", at(time.Second))
	if got, ok := l.Expire(expiry, at(TTL)).Current(at(TTL)); !ok || got.Text != "second" {
		t.Fatalf("an expiry from before Clear touched the next message: %+v, %v", got, ok)
	}
}

func TestSetCleansTheText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "unknown command: nope", want: "unknown command: nope"},
		{name: "newlines fold into one row", in: "exec: exit status 1\nstderr:\n  boom", want: "exec: exit status 1 stderr: boom"},
		{name: "escape sequences are dropped", in: "\x1b[31mred\x1b[0m text", want: "red text"},
		{name: "control characters become spaces", in: "a\x07b\tc", want: "a b c"},
		{name: "surrounding blanks go", in: "  padded  ", want: "padded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, _ := set(Info, tc.in).Current(t0)
			if got.Text != tc.want {
				t.Fatalf("Text = %q, want %q", got.Text, tc.want)
			}
		})
	}
}

func TestRedact(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		home string
		in   string
		want string
	}{
		{name: "path under home", home: "/home/al", in: "open /home/al/.kube/config: denied", want: "open ~/.kube/config: denied"},
		{name: "home itself", home: "/home/al", in: "stat /home/al: no such file", want: "stat ~: no such file"},
		{name: "home at the end", home: "/home/al", in: "HOME is /home/al", want: "HOME is ~"},
		{name: "quoted", home: "/home/al", in: `read "/home/al/x"`, want: `read "~/x"`},
		{name: "every occurrence", home: "/home/al", in: "/home/al/a and /home/al/b", want: "~/a and ~/b"},
		{name: "trailing separator on home", home: "/home/al/", in: "/home/al/x", want: "~/x"},
		{name: "longer user name is left alone", home: "/home/al", in: "/home/alice/x", want: "/home/alice/x"},
		{name: "home inside another path is left alone", home: "/home/al", in: "/backup/home/al/x", want: "/backup/home/al/x"},
		{name: "home repeated inside another path", home: "/home/al", in: "/backup/home/al/home/al", want: "/backup/home/al/home/al"},
		{name: "adjacent occurrences redact only the whole path", home: "/home/al", in: "/home/al/home/al", want: "~/home/al"},
		{name: "separated occurrences", home: "/home/al", in: "/home/al /home/al", want: "~ ~"},
		{name: "windows home", home: `C:\Users\al`, in: `open C:\Users\al\.kube\config`, want: `open ~\.kube\config`},
		{name: "no home", home: "", in: "/home/al/x", want: "/home/al/x"},
		{name: "root home is not a prefix of everything", home: "/", in: "/etc/rancher/k3s/k3s.yaml", want: "/etc/rancher/k3s/k3s.yaml"},
		{name: "nothing to redact", home: "/home/al", in: "service web has no selector", want: "service web has no selector"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := redact(tc.in, tc.home); got != tc.want {
				t.Fatalf("redact(%q, %q) = %q, want %q", tc.in, tc.home, got, tc.want)
			}
		})
	}
}

// TestSetRedactsTheHomeDirectory sets HOME, so it cannot run in parallel.
func TestSetRedactsTheHomeDirectory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	got, _ := set(Error, "load kubeconfig: open "+home+"/.kube/config: permission denied").Current(t0)
	if want := "load kubeconfig: open ~/.kube/config: permission denied"; got.Text != want {
		t.Fatalf("Text = %q, want %q", got.Text, want)
	}
}
