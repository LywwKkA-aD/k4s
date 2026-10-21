package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// clearInput empties a prompt whose cursor sits at the end of the text.
func clearInput() tea.Msg { return tuitest.Press("ctrl+u") }

func tailRequest() views.TailPromptRequestMsg {
	return views.TailPromptRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}, Container: tuitest.ContainerNginx,
	}
}

func TestTailPrompt(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		keys     []tea.Msg
		wantTail int64 // 0: no logs request
	}{
		{name: "enter takes the default", keys: tuitest.Keys("enter"), wantTail: 100},
		{name: "custom count", keys: []tea.Msg{clearInput(), tuitest.Type("250"), tuitest.Enter()}, wantTail: 250},
		{name: "garbage falls back to the default", keys: []tea.Msg{clearInput(), tuitest.Type("lots"), tuitest.Enter()}, wantTail: 100},
		{name: "zero falls back to the default", keys: []tea.Msg{clearInput(), tuitest.Type("0"), tuitest.Enter()}, wantTail: 100},
		{name: "esc cancels", keys: tuitest.Keys("esc")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tailRequest())
			requireScreen(t, m, "tail lines · pod/web-1 · ns demo", "Enter run · Esc cancel · default 100", "❯ 100")

			m, out := tuitest.Drive(t, m, tc.keys...)
			logs := tuitest.All[views.LogsRequestMsg](out)
			if tc.wantTail == 0 {
				if len(logs) != 0 {
					t.Fatalf("cancelled prompt opened logs: %+v", logs)
				}
				requireView(t, m, "dashboard")
				refuteScreen(t, m, "tail lines")
				return
			}
			want := views.LogsRequestMsg{
				Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1},
				Tail: tc.wantTail, Container: tuitest.ContainerNginx,
			}
			if len(logs) != 1 || !tuitest.Contains(out, want) {
				t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
			}
			requireView(t, m, "logs · web-1 / nginx")
		})
	}
}

func TestPromptsCaptureGlobalKeys(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tailRequest())
	m, out := tuitest.Drive(t, m, tuitest.Press("q"), tuitest.Press(":"), tuitest.Press("?"))
	if hasQuit(out) {
		t.Fatal("q inside the tail prompt quit the app")
	}
	requireScreen(t, m, "❯ 100q:?")
	refuteScreen(t, m, "global")
}

func containerRequest(next string, pods ...string) views.ContainerPromptRequestMsg {
	return views.ContainerPromptRequestMsg{
		Namespace:  tuitest.NamespaceDemo,
		Pods:       pods,
		Containers: []string{tuitest.ContainerApp, tuitest.ContainerSidecar, "init"},
		NextKind:   next,
	}
}

func TestContainerPicker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []string
		want string // picked container; "" means none picked
	}{
		{name: "enter picks the first", keys: []string{"enter"}, want: tuitest.ContainerApp},
		{name: "down moves", keys: []string{"down", "enter"}, want: tuitest.ContainerSidecar},
		{name: "j moves", keys: []string{"j", "j", "enter"}, want: "init"},
		{name: "up wraps to the last", keys: []string{"up", "enter"}, want: "init"},
		{name: "k wraps to the last", keys: []string{"k", "enter"}, want: "init"},
		{name: "down wraps to the first", keys: []string{"down", "down", "down", "enter"}, want: tuitest.ContainerApp},
		{name: "digit picks at once", keys: []string{"2"}, want: tuitest.ContainerSidecar},
		{name: "digit out of range is ignored", keys: []string{"9"}},
		{name: "other keys are ignored", keys: []string{"x", "q"}},
		{name: "esc cancels", keys: []string{"esc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), containerRequest("logs", tuitest.PodMulti))
			requireScreen(t, m, "container", "▶ 1. app", "2. sidecar", "3. init", "1-9 quick")

			m, out := tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			tails := tuitest.All[views.TailPromptRequestMsg](out)
			if tc.want == "" {
				if len(tails) != 0 || hasQuit(out) {
					t.Fatalf("no pick expected, got:\n%s", tuitest.Describe(out))
				}
				return
			}
			want := views.TailPromptRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodMulti}, Container: tc.want}
			if len(tails) != 1 || !tuitest.Contains(out, want) {
				t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
			}
			// The pick chains into the tail prompt, which names the pod.
			requireScreen(t, m, "tail lines · pod/multi-0 · ns demo", "❯ 100")
		})
	}
}

func TestContainerPickerStaysOpenOnIgnoredKeys(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), containerRequest("logs", tuitest.PodMulti))
	m, _ = tuitest.Drive(t, m, tuitest.Keys("9", "x", "down")...)
	requireScreen(t, m, "▶ 2. sidecar")
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	refuteScreen(t, m, "1-9 quick")
	requireView(t, m, "dashboard")
}

func TestContainerPickerForExec(t *testing.T) {
	t.Parallel()

	_, out := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)),
		containerRequest("exec", tuitest.PodMulti, "ignored-second-pod"), tuitest.Press("2"))
	want := views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodMulti, Container: tuitest.ContainerSidecar}
	if !tuitest.Contains(out, want) {
		t.Errorf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
}

func TestContainerPickerWithNothingToDo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  views.ContainerPromptRequestMsg
		keys []string
	}{
		{name: "no containers opens nothing", msg: views.ContainerPromptRequestMsg{Namespace: "demo", Pods: []string{"p"}, NextKind: "logs"}},
		{name: "exec without a pod", msg: containerRequest("exec"), keys: []string{"enter"}},
		{name: "unknown follow-up", msg: containerRequest("attach", tuitest.PodMulti), keys: []string{"enter"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.msg)
			m, more := tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			if reqs := tuitest.Requests(append(out, more...)); len(reqs) != 0 {
				t.Errorf("emitted requests:\n%s", tuitest.Describe(reqs))
			}
			refuteScreen(t, m, "1-9 quick")
			requireView(t, m, "dashboard")
		})
	}
}

func TestHelpPopup(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup []tea.Msg
		want  []string
		not   []string
	}{
		{
			name: "dashboard",
			want: []string{"help", "global", "force quit", "command bar", "commands", ":pods", "po, pod", ":forwards", ":ns <name>", ":<list> <ns>", "? / Esc close", "view: dashboard"},
		},
		{
			name:  "pods view lists its own keys",
			setup: colon("pods"),
			want:  []string{"view: pods", "describe", "port-forward", "home (then quit on dashboard)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.setup...)
			m, _ = tuitest.Drive(t, m, tuitest.Press("?"))
			requireScreen(t, m, tc.want...)
			m, out := tuitest.Drive(t, m, tuitest.Press("q"))
			if hasQuit(out) {
				t.Fatal("the key that closes help must not also act")
			}
			refuteScreen(t, m, "force quit")
		})
	}
}

func TestForwardPromptOpensPrefilled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  views.ForwardRequestMsg
		want []string
	}{
		{
			name: "privileged remote port gets a high local port",
			msg:  views.ForwardRequestMsg{Kind: "service", Namespace: tuitest.NamespaceDemo, Name: tuitest.ServiceWeb, RemotePort: 80},
			want: []string{"port-forward", "service/web · demo", "❯ 8080:80", "format local:remote"},
		},
		{
			name: "high remote port is kept",
			msg:  views.ForwardRequestMsg{Kind: "service", Namespace: tuitest.NamespaceDemo, Name: tuitest.ServiceMulti, RemotePort: 8443},
			want: []string{"service/multi · demo", "❯ 8443:8443"},
		},
		{
			name: "no hint shows a placeholder",
			msg:  views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1},
			want: []string{"pod/web-1 · demo", "8080:80"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.msg)
			requireScreen(t, m, tc.want...)
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			refuteScreen(t, m, "format local:remote")
			requireView(t, m, "dashboard")
		})
	}
}

// TestForwardPromptKeepsBadInputOpen: a typo in the pair keeps the popup
// open with the reason in it, so the user can fix it in place.
func TestForwardPromptKeepsBadInputOpen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		hint  uint16
		input string
		want  string
	}{
		{name: "empty without a hint", input: "", want: "port required (try 8080:80)"},
		{name: "not a number", hint: 80, input: "abc", want: `"abc" is not a port number`},
		{name: "zero", hint: 80, input: "0", want: "port must be > 0"},
		{name: "out of range", hint: 80, input: "70000", want: `"70000" is not a port number`},
		{name: "both sides empty", hint: 80, input: ":", want: "both ports empty"},
		{name: "bad local side", hint: 80, input: "x:80", want: `local port: "x" is not a port number`},
		{name: "bad remote side", hint: 80, input: "8080:y", want: `remote port: "y" is not a port number`},
		{name: "three parts", hint: 80, input: "1:2:3", want: `remote port: "2:3" is not a port number`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := views.ForwardRequestMsg{Kind: "service", Namespace: tuitest.NamespaceDemo, Name: tuitest.ServiceWeb, RemotePort: tc.hint}
			keys := []tea.Msg{req, clearInput()}
			if tc.input != "" {
				keys = append(keys, tuitest.Type(tc.input))
			}
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), append(keys, tuitest.Enter())...)
			requireScreen(t, m, "port-forward", tc.want, "format local:remote")
			if got := footer(m); strings.Contains(got, tc.want) {
				t.Errorf("the error went to the footer too: %q", got)
			}
			requireView(t, m, "dashboard")

			// Esc drops the prompt and its error; reopening starts clean.
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			refuteScreen(t, m, "format local:remote", tc.want)
			m, _ = tuitest.Drive(t, m, req)
			refuteScreen(t, m, tc.want)
		})
	}
}
