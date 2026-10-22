package views_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func podRefs(names ...string) []k8s.PodRef {
	out := make([]k8s.PodRef, 0, len(names))
	for _, n := range names {
		out = append(out, k8s.PodRef{Name: n, Phase: "Running"})
	}
	return out
}

func finishedPods(n int) []k8s.SkippedPod {
	out := make([]k8s.SkippedPod, 0, n)
	for range n {
		out = append(out, k8s.SkippedPod{Phase: "Succeeded", Reason: k8s.SkipFinished, InSelector: true})
	}
	return out
}

func TestLogsCommand(t *testing.T) {
	t.Parallel()

	deploy := k8s.LogTarget{
		Kind: k8s.LogKindDeployment, Namespace: "demo", Name: "web",
		Selector: "app=web", Hashes: []string{"h2", "h1"}, Pods: podRefs("web-a", "web-b"),
	}
	service := k8s.LogTarget{
		Kind: k8s.LogKindService, Namespace: "demo", Name: "web",
		Selector: "app=web", Pods: podRefs("web-a", "web-b"), Skipped: finishedPods(13),
	}
	guessed := deploy
	guessed.OwnersGuessed = true
	expressions := deploy
	expressions.Selector = "tier in (a,b)"
	expressions.Hashes = nil

	cases := []struct {
		name      string
		pods      []string
		container string
		tail      int64
		target    k8s.LogTarget
		want      string
		approx    bool
	}{
		{
			name: "one pod", pods: []string{"web-a"}, tail: 100,
			want: "kubectl logs -f web-a --tail=100 -n demo",
		},
		{
			name: "one pod and a container", pods: []string{"web-a"}, container: "app", tail: 20,
			want: "kubectl logs -f web-a --tail=20 -n demo -c app",
		},
		{
			name: "one pod of a resolved deployment names the pod", pods: []string{"web-a"}, tail: 100, target: guessed,
			want: "kubectl logs -f web-a --tail=100 -n demo",
		},
		{
			name: "deployment narrows to its ReplicaSets", pods: []string{"web-a", "web-b"}, tail: 50, target: deploy,
			want: "kubectl logs -f -l 'app=web,pod-template-hash in (h2,h1)' --prefix --tail=50 --max-log-requests=2 -n demo",
		},
		{
			name: "deployment with a container", pods: []string{"web-a", "web-b"}, container: "app", tail: 50, target: deploy,
			want: "kubectl logs -f -l 'app=web,pod-template-hash in (h2,h1)' --prefix --tail=50 --max-log-requests=2 -n demo -c app",
		},
		{
			name: "deployment with guessed owners", pods: []string{"web-a", "web-b"}, tail: 50, target: guessed,
			want:   "kubectl logs -f -l 'app=web,pod-template-hash in (h2,h1)' --prefix --tail=50 --max-log-requests=2 -n demo",
			approx: true,
		},
		{
			name: "set-based selector", pods: []string{"web-a", "web-b"}, tail: 10, target: expressions,
			want: "kubectl logs -f -l 'tier in (a,b)' --prefix --tail=10 --max-log-requests=2 -n demo",
		},
		{
			name: "service skipping finished pods", pods: []string{"web-a", "web-b"}, tail: 100, target: service,
			want:   "kubectl logs -f -l app=web --prefix --tail=100 --max-log-requests=15 -n demo",
			approx: true,
		},
		{
			name: "several pods without a target follow the first", pods: []string{"web-a", "web-b"}, tail: 100,
			want:   "kubectl logs -f web-a --tail=100 -n demo",
			approx: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := views.LogsCommand("demo", tc.pods, tc.container, tc.tail, tc.target)
			if got.String() != tc.want {
				t.Errorf("String() = %q\nwant       %q", got.String(), tc.want)
			}
			if got.Approx != tc.approx {
				t.Errorf("Approx = %v, want %v", got.Approx, tc.approx)
			}
			if got.Mutating {
				t.Error("a logs command is not mutating")
			}
			wantDisplay := tc.want
			if tc.approx {
				wantDisplay = kubectl.ApproxMark + " " + tc.want
			}
			if got.Display() != wantDisplay {
				t.Errorf("Display() = %q, want %q", got.Display(), wantDisplay)
			}
		})
	}
}

// TestLogsCommandWithoutAValidCommand: anything kubectl would reject
// renders as an empty footer row, never a panic.
func TestLogsCommandWithoutAValidCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		namespace string
		pods      []string
		container string
		target    k8s.LogTarget
	}{
		{name: "no pods", namespace: "demo"},
		{name: "bad namespace", namespace: "Not_A_Namespace", pods: []string{"web-a"}},
		{name: "bad pod name", namespace: "demo", pods: []string{"-rf"}},
		{name: "bad container", namespace: "demo", pods: []string{"web-a"}, container: "--all"},
		{
			name: "bad namespace with a target", namespace: "", pods: []string{"a", "b"},
			target: k8s.LogTarget{Kind: k8s.LogKindService, Selector: "app=web"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := views.LogsCommand(tc.namespace, tc.pods, tc.container, 100, tc.target)
			if !got.IsZero() || got.Display() != "" {
				t.Errorf("LogsCommand = %q, want the zero Cmd", got.Display())
			}
		})
	}
}

func TestExecCommand(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		req     views.ExecRequestMsg
		want    string
		wantErr bool
	}{
		{name: "default container", req: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1"}, want: "kubectl exec -it -n demo web-1 -- sh"},
		{name: "named container", req: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1", Container: "app"}, want: "kubectl exec -it -n demo -c app web-1 -- sh"},
		{name: "a linux pod", req: views.ExecRequestMsg{Namespace: "demo", Pod: "web-1", Shell: views.ExecShell}, want: "kubectl exec -it -n demo web-1 -- sh"},
		{
			name: "a windows pod", req: views.ExecRequestMsg{Namespace: "demo", Pod: "iis-1", Container: "web", Shell: views.WindowsShell},
			want: "kubectl exec -it -n demo -c web iis-1 -- cmd.exe",
		},
		{name: "pod named like a flag", req: views.ExecRequestMsg{Namespace: "demo", Pod: "--all"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := views.ExecCommand(tc.req)
			if tc.wantErr {
				if !errors.Is(err, kubectl.ErrInvalidName) {
					t.Fatalf("err = %v, want ErrInvalidName", err)
				}
				return
			}
			if err != nil || got.String() != tc.want {
				t.Fatalf("ExecCommand = %q, %v; want %q", got.String(), err, tc.want)
			}
		})
	}
}

func TestShellFor(t *testing.T) {
	t.Parallel()

	if got := views.ShellFor(false); got != "sh" {
		t.Errorf("ShellFor(linux) = %q, want sh", got)
	}
	if got := views.ShellFor(true); got != "cmd.exe" {
		t.Errorf("ShellFor(windows) = %q, want cmd.exe", got)
	}
}

func TestStreamedPods(t *testing.T) {
	t.Parallel()

	for n, want := range map[int]int{0: 0, 1: 1, 50: 50, 73: kubectl.MaxLogRequests} {
		if got := views.StreamedPods(n); got != want {
			t.Errorf("StreamedPods(%d) = %d, want %d", n, got, want)
		}
	}
}

// TestLogsCommandDoesNotShareThePodSlice: the builder reads pods and never
// keeps them, so a view that reuses its slice cannot change a built Cmd.
func TestLogsCommandDoesNotShareThePodSlice(t *testing.T) {
	t.Parallel()

	pods := []string{"web-a"}
	got := views.LogsCommand("demo", pods, "", 100, k8s.LogTarget{})
	pods[0] = "web-z"
	if strings.Contains(got.String(), "web-z") {
		t.Errorf("Cmd follows the caller's slice: %q", got.String())
	}
}
