package tui_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// clipboard returns what the messages asked the terminal to put on the
// clipboard: the payload of tea.SetClipboard's message.
func clipboard(msgs []tea.Msg) (string, bool) {
	for _, msg := range msgs {
		if fmt.Sprintf("%T", msg) == "tea.setClipboardMsg" {
			return reflect.ValueOf(msg).String(), true
		}
	}
	return "", false
}

// TestCopyTakesTheFooterCommand: y copies what the footer shows, as it
// would be pasted: with --context after a switch, never with the
// approximate mark.
func TestCopyTakesTheFooterCommand(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name  string
		setup []tea.Msg
		want  string
	}{
		{name: "list view", setup: colon("pods"), want: "kubectl get pods -A"},
		{
			name:  "after a context switch",
			setup: then([]tea.Msg{views.ContextSelectedMsg{Name: "other"}}, colon("pods")),
			want:  "kubectl --context other get pods -n demo",
		},
		{
			name:  "approximate command",
			setup: []tea.Msg{views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Tail: 50}},
			want:  "kubectl logs -f web-1 --tail=50 -n demo",
		},
		{
			name: "container picker copies its own command",
			setup: []tea.Msg{views.ContainerPromptRequestMsg{
				Namespace: demo, Pods: []string{tuitest.PodMulti},
				Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar}, NextKind: "exec",
			}},
			want: "kubectl exec -it -n demo -c app multi-0 -- sh",
		},
		{name: "help copies the view's command", setup: then(colon("svc"), tuitest.Keys("?")), want: "kubectl get services -A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.setup...)
			before := tuitest.Screen(m)
			m, out := tuitest.Drive(t, m, tuitest.Press("y"))
			got, ok := clipboard(out)
			if !ok || got != tc.want {
				t.Fatalf("clipboard = %q, %v; want %q\n%s", got, ok, tc.want, tuitest.Describe(out))
			}
			if row := statusRow(footer(m)); row != "copied: "+tc.want {
				t.Errorf("status row = %q", row)
			}
			// The overlay or view underneath stays as it was.
			if body, after := bodyOf(before), bodyOf(tuitest.Screen(m)); body != after {
				t.Errorf("y changed the screen:\n%s\n---\n%s", body, after)
			}
		})
	}
}

// bodyOf is a screen without its footer.
func bodyOf(screen string) string {
	ls := strings.Split(screen, "\n")
	for i := len(ls) - 1; i >= 0; i-- {
		if isRule(ls[i]) {
			return strings.Join(ls[:i], "\n")
		}
	}
	return screen
}

// TestCopyIsTextWhereTextIsTyped: in a prompt with an input, and while a
// view owns the keyboard, y is a letter.
func TestCopyIsTextWhereTextIsTyped(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup []tea.Msg
		want  string // on screen after y
	}{
		{name: "tail prompt", setup: []tea.Msg{tailRequest()}, want: "❯ 100y"},
		{name: "command bar", setup: tuitest.Keys(":"), want: ": y"},
		{name: "port prompt", setup: []tea.Msg{views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1, RemotePort: 80}}, want: "❯ 8080:80y"},
		{name: "list filter", setup: then(colon("pods"), tuitest.Keys("/")), want: "/ y"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.setup...)
			m, out := tuitest.Drive(t, m, tuitest.Press("y"))
			if got, ok := clipboard(out); ok {
				t.Fatalf("y copied %q", got)
			}
			requireScreen(t, m, tc.want)
		})
	}
}

func TestCopyWithNothingToCopy(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon("pf")...)
	m, out := tuitest.Drive(t, m, tuitest.Press("y"))
	if got, ok := clipboard(out); ok {
		t.Fatalf("y copied %q from an empty forwards list", got)
	}
	if row := statusRow(footer(m)); row != "nothing to copy" {
		t.Errorf("status row = %q", row)
	}
}

// TestCopyShowsAShortenedHome: the clipboard gets the real path, the
// status row the one with ~.
func TestCopyShowsAShortenedHome(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, "clusters", "lab.yaml")
	c := tuitest.NewFakeClient(t)
	c.Source = k8s.Source{Paths: []string{path}, Origin: k8s.OriginFlag}
	c.KubeconfigPaths = []string{path}
	m, out := tuitest.Drive(t, startedWith(t, c, nil), tuitest.Press("y"))
	want := "kubectl --kubeconfig " + path + " get all -A"
	if got, _ := clipboard(out); got != want {
		t.Fatalf("clipboard = %q, want %q", got, want)
	}
	if row := statusRow(footer(m)); row != "copied: kubectl --kubeconfig ~/clusters/lab.yaml get all -A" {
		t.Errorf("status row = %q", row)
	}
}

func TestCopyIsListedInHintsAndHelp(t *testing.T) {
	t.Parallel()

	m := started(t, tuitest.NewFakeClient(t))
	if row := statusRow(footer(m)); !strings.Contains(row, "? help  ·  y copy") {
		t.Errorf("hints row = %q", row)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("?"))
	requireScreen(t, m, "copy the kubectl command", "set -g set-clipboard on")
}
