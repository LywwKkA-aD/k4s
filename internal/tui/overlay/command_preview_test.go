package overlay_test

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// webTarget is a deployment resolved to two pods of one ReplicaSet.
func webTarget() k8s.LogTarget {
	return k8s.LogTarget{
		Kind: k8s.LogKindDeployment, Namespace: "demo", Name: "web", Selector: "app=web",
		Hashes: []string{"h1"}, Pods: []k8s.PodRef{{Name: "web-1"}, {Name: "web-2"}},
	}
}

// TestTailPromptPreviewsTheLogsCommand: the footer under the tail prompt
// already shows the stream Enter would open, at the count typed so far.
func TestTailPromptPreviewsTheLogsCommand(t *testing.T) {
	t.Parallel()

	clear := tuitest.Press("ctrl+u")
	deploy := views.TailPromptRequestMsg{
		Namespace: "demo", Pods: []string{"web-1", "web-2"}, Container: "app",
		Target: views.ResourceRef{Kind: "deployment", Namespace: "demo", Name: "web"}, Logs: webTarget(),
	}
	cases := []struct {
		name  string
		req   views.TailPromptRequestMsg
		input []tea.Msg
		want  string
	}{
		{
			name: "one pod at the default", req: views.TailPromptRequestMsg{Namespace: "demo", Pods: []string{"web-1"}},
			want: "kubectl logs -f web-1 --tail=100 -n demo",
		},
		{
			name: "deployment at a typed count", req: deploy, input: []tea.Msg{clear, tuitest.Type("250")},
			want: "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=250 --max-log-requests=2 -n demo -c app",
		},
		{
			name: "a count over the cap previews the clamped tail", req: deploy, input: []tea.Msg{clear, tuitest.Type("100000")},
			want: "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=2500 --max-log-requests=2 -n demo -c app",
		},
		{
			name: "garbage previews the default", req: deploy, input: []tea.Msg{clear, tuitest.Type("lots")},
			want: "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=100 --max-log-requests=2 -n demo -c app",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, _ := feed(t, overlay.NewTail(tc.req), tc.input...)
			got, ok := overlay.CommandOf(o)
			if !ok || got.String() != tc.want {
				t.Fatalf("CommandOf = %q, %v\nwant          %q", got.String(), ok, tc.want)
			}
		})
	}
}

func TestContainerPickerPreviewsTheNextCommand(t *testing.T) {
	t.Parallel()

	logs := pickerRequest()
	exec := pickerRequest()
	exec.NextKind = "exec"
	windows := pickerRequest()
	windows.NextKind, windows.Shell = "exec", views.WindowsShell
	deploy := views.ContainerPromptRequestMsg{
		Namespace: "demo", Pods: []string{"web-1", "web-2"}, Containers: []string{"app", "sidecar"},
		NextKind: "logs", Logs: webTarget(),
	}
	noPod := pickerRequest()
	noPod.NextKind, noPod.Pods = "exec", nil
	other := pickerRequest()
	other.NextKind = "attach"
	badPod := pickerRequest()
	badPod.NextKind, badPod.Pods = "exec", []string{"-rf"}
	empty := views.ContainerPromptRequestMsg{Namespace: "demo", Pods: []string{"multi-0"}, NextKind: "logs"}

	cases := []struct {
		name string
		req  views.ContainerPromptRequestMsg
		keys []string
		want string // "" means no command
	}{
		{name: "logs of the highlighted container", req: logs, keys: []string{"down"}, want: "kubectl logs -f multi-0 --tail=100 -n demo -c sidecar"},
		{name: "exec into the highlighted container", req: exec, want: "kubectl exec -it -n demo -c app multi-0 -- sh"},
		{name: "exec into a windows pod", req: windows, keys: []string{"down"}, want: "kubectl exec -it -n demo -c sidecar multi-0 -- cmd.exe"},
		{name: "deployment logs", req: deploy, want: "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=100 --max-log-requests=2 -n demo -c app"},
		{name: "exec without a pod", req: noPod},
		{name: "unknown follow-up", req: other},
		{name: "exec into an invalid pod name", req: badPod},
		{name: "no containers", req: empty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, _ := feed(t, overlay.NewContainer(tc.req), keys(tc.keys...)...)
			got, ok := overlay.CommandOf(o)
			if !ok || got.String() != tc.want {
				t.Fatalf("CommandOf = %q, %v; want %q", got.String(), ok, tc.want)
			}
		})
	}
}

func TestOverlaysWithoutACommand(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Time{} }
	for _, o := range []overlay.Overlay{
		nil,
		overlay.NewCommand(),
		overlay.NewHelp(nil),
		overlay.NewRestore(1, now),
		overlay.NewForward(views.ForwardRequestMsg{Kind: "pod", Name: "p"}, nil),
	} {
		if got, ok := overlay.CommandOf(o); ok || !got.IsZero() {
			t.Errorf("%T: CommandOf = %q, %v; want none", o, got.String(), ok)
		}
	}
}

// TestTakesText: y is text in a prompt with an input and a copy key
// everywhere else.
func TestTakesText(t *testing.T) {
	t.Parallel()

	now := func() time.Time { return time.Time{} }
	cases := []struct {
		name string
		o    overlay.Overlay
		want bool
	}{
		{name: "none", o: nil},
		{name: "command bar", o: overlay.NewCommand(), want: true},
		{name: "tail", o: overlay.NewTail(views.TailPromptRequestMsg{Pods: []string{"p"}}), want: true},
		{name: "forward", o: overlay.NewForward(views.ForwardRequestMsg{Kind: "pod", Name: "p"}, nil), want: true},
		{name: "container", o: overlay.NewContainer(pickerRequest())},
		{name: "restore", o: overlay.NewRestore(1, now)},
		{name: "help", o: overlay.NewHelp(nil)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := overlay.TakesText(tc.o); got != tc.want {
				t.Fatalf("TakesText = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTailPromptMarksADetachedTarget: when the cursor left the row the
// logs were asked for, the prompt still opens but says so.
func TestTailPromptMarksADetachedTarget(t *testing.T) {
	t.Parallel()

	req := views.TailPromptRequestMsg{
		Namespace: "demo", Pods: []string{"web-1", "web-2"},
		Target: views.ResourceRef{Kind: "deployment", Namespace: "demo", Name: "web"}, Detached: true,
	}
	o := overlay.NewTail(req)
	requireShows(t, o, "tail lines · deployment/web · ns demo", "(not the selected row)", "❯ 100")
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
	req.Detached = false
	if strings.Contains(screen(overlay.NewTail(req)), "not the selected row") {
		t.Error("an attached request is marked")
	}
}

// The preview never carries --context or --kubeconfig: the root adds those.
func TestPreviewIsUndecorated(t *testing.T) {
	t.Parallel()

	got, _ := overlay.CommandOf(overlay.NewTail(views.TailPromptRequestMsg{Namespace: "demo", Pods: []string{"web-1"}}))
	want := kubectl.New("logs", "-f", "web-1", "--tail=100", "-n", "demo")
	if got.String() != want.String() {
		t.Fatalf("preview = %q, want %q", got.String(), want.String())
	}
}
