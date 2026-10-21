package dashboard_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// hostile is the busy fixture with escape sequences in every kind of
// cluster text the first three panels and the banner draw.
func hostile() dash.Snapshot {
	s := busyK3s()
	osc52 := "\x1b]52;c;ZXZpbA==\a"
	s.Cluster.You.Text = "dev" + osc52 + "ops"
	s.Cluster.Version.GitVersion = "v1.36.3\x1b[2J+k3s1"
	s.Cluster.Server.Text = "https://10.0.0.5:6443\x1b]8;;http://evil\x1b\\"
	s.Nodes.Rows[0].Key.Name = "k3s-cp-1\x1b[H"
	s.Pods.Problems[0].Key.Name = "payments" + osc52 + "-worker"
	s.Pods.Problems[0].Status = "Crash\x1b[31mLoop"
	s.Pods.Problems[0].Cmd = dash.Cmd{Args: []string{"describe", "pod", "payments" + osc52 + "-worker", "-n", "shop"}}
	s.Verdict.Chips[0].Text = "3 Crash" + osc52
	s.Next[0].Question = "why?" + osc52
	s.Workloads.Rows[0].Key.Name = "payments" + osc52 + "-worker"
	s.Events.Rows[0].Reason = "Back\x1b[2JOff"
	s.Events.Rows[0].Message = "Back-off" + osc52 + " restarting failed container"
	s.Events.Rows[0].Object.Name = "loki\x1b[H-backend-0"
	return s
}

// TestClusterTextIsInert is §10.7: cluster strings carrying OSC 52 and
// other escape sequences render as plain text, in the body and in the
// footer command.
func TestClusterTextIsInert(t *testing.T) {
	t.Parallel()

	m, _ := press(t, mount(t, newFeed(hostile), wideW, wideH), "3", "down")
	frame := tuitest.Frame(m)
	for _, bad := range []string{"\x1b]", "\a", "\x1b[2J", "\x1b[H", "\x1b\\"} {
		if i := strings.Index(frame, bad); i >= 0 {
			t.Fatalf("the frame carries %q: %q", bad, frame[max(i-80, 0):min(i+40, len(frame))])
		}
	}
	screen := tuitest.Screen(m)
	for _, want := range []string{"devops", "payments-worker", "CrashLoop", "v1.36.3+k3s1", "shop/payments-worker", "BackOff", "Back-off restarting", "pod/loki-backend-0"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks the cleaned %q", want)
		}
	}
	if got := command(m); strings.ContainsAny(got, "\x1b\a") || !strings.Contains(got, "payments-worker") {
		t.Fatalf("footer command = %q, want it cleaned", got)
	}
}
