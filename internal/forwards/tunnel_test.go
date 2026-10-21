package forwards

import (
	"context"
	"errors"
	"testing"
	"time"
)

// kubeletRefused is the kubelet's message when nothing listens on the
// remote port.
const kubeletRefused = `error forwarding port 80 to pod 4c1f, uid : failed to execute portforward in network namespace "/var/run/netns/cni-1": failed to connect to localhost:80 inside namespace "4c1f", IPv4: dial tcp4 127.0.0.1:80: connect: connection refused IPv6 dial tcp6 [::1]:80: connect: connection refused`

func TestTunnelCause(t *testing.T) {
	t.Parallel()
	cases := []struct {
		msg, want string
	}{
		{msg: kubeletRefused, want: "connection refused"},
		{msg: "read tcp 10.42.0.1:443->10.42.0.9:51234: read: connection reset by peer", want: "connection reset by peer"},
		{msg: "dial tcp 10.42.0.9:80: i/o timeout", want: "i/o timeout"},
		{msg: "timed out waiting for the condition", want: "timed out waiting for the condition"},
		{msg: "error forwarding port 80 to pod 4c1f: something new: the real reason", want: "the real reason"},
		{msg: "plain", want: "plain"},
		{msg: "  ends with a colon: ", want: "ends with a colon:"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if got := tunnelCause(tc.msg); got != tc.want {
				t.Errorf("tunnelCause(%q) = %q, want %q", tc.msg, got, tc.want)
			}
		})
	}
}

// client-go drops the whole session once the kubelet reports an error on a
// connection: the forward ends with "lost connection to pod", and the list
// keeps the reason until the forward is started again.
func TestATunnelErrorOutlivesTheSessionItEnded(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))
	sess := running(t, m, fake, "f1")[0]

	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	sess.failConnection(kubeletRefused, at)
	sess.Err <- errors.New("lost connection to pod")
	sess.finish()
	a := waitStatus(t, m, "f1", StatusError)
	if a.TunnelErr != "connection refused" || !a.TunnelErrAt.Equal(at) {
		t.Errorf("listed %+v, want the refused connection at %v", a, at)
	}

	done := startAsync(context.Background(), m, "f1")
	restarted := fake.next(t)
	if a := statusOf(t, m, "f1"); a.TunnelErr != "" {
		t.Errorf("a restart still shows %q", a.TunnelErr)
	}
	restarted.ready()
	if err := awaitErr(t, done); err != nil {
		t.Fatalf("Start: %v", err)
	}
}

// A running forward whose connections fail keeps running; the list shows
// the latest failure and when it happened, and stops showing it once the
// forward is stopped.
func TestATunnelErrorShowsWhileTheForwardRuns(t *testing.T) {
	t.Parallel()
	fake := newFakeTunneler(t)
	m := newTestManager(t, deps{tunnel: fake})
	mustRegister(t, m, testForward("f1", "pod", 18080))
	sess := running(t, m, fake, "f1")[0]
	if a := statusOf(t, m, "f1"); a.TunnelErr != "" || !a.TunnelErrAt.IsZero() {
		t.Fatalf("a fresh forward shows a tunnel error: %+v", a)
	}

	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	changed := m.Changes()
	sess.failConnection(kubeletRefused, at)
	select {
	case <-changed:
	case <-time.After(testTimeout):
		t.Fatal("a tunnel error did not refresh the list")
	}
	a := statusOf(t, m, "f1")
	if a.Status != StatusRunning || a.TunnelErr != "connection refused" || !a.TunnelErrAt.Equal(at) {
		t.Errorf("listed %+v, want running with the refused connection at %v", a, at)
	}

	if err := m.Stop("f1"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if a := statusOf(t, m, "f1"); a.TunnelErr != "" {
		t.Errorf("a stopped forward still shows %q", a.TunnelErr)
	}
}
