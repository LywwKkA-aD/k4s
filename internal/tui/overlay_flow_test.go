package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Help closes on ?, Esc or q and does nothing else with them; any other key
// closes it and then does what it does without help open.
func TestHelpPassesOtherKeysOn(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		key      string
		view     string
		footer   string // the footer contains it
		describe bool   // the key opened describe on the row
	}{
		{name: "question mark only closes", key: "?", view: "pods", footer: "q home"},
		{name: "q only closes", key: "q", view: "pods", footer: "q home"},
		{name: "esc only closes", key: "esc", view: "pods", footer: "q home"},
		{name: "colon opens the command bar", key: ":", view: "pods", footer: ": pods, ns, dashboard"},
		{name: "slash opens the filter", key: "/", view: "pods", footer: "kubectl get pods -A\n"},
		{name: "enter opens describe", key: "enter", describe: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys("?"))...)
			requireScreen(t, m, "force quit")

			m, out := tuitest.Drive(t, m, tuitest.Press(tc.key))
			refuteScreen(t, m, "force quit")
			if hasQuit(out) {
				t.Fatal("a key that closes help quit the app")
			}
			if _, ok := tuitest.Find[views.DescribeRequestMsg](out); ok != tc.describe {
				t.Fatalf("describe requested = %v, want %v:\n%s", ok, tc.describe, tuitest.Describe(out))
			}
			if tc.describe {
				return
			}
			requireView(t, m, tc.view)
			if got := footer(m) + "\n"; !strings.Contains(got, tc.footer) {
				t.Fatalf("footer lacks %q:\n%s", tc.footer, got)
			}
			if tc.key == "/" {
				m, _ = tuitest.Drive(t, m, tuitest.Type("web"))
				requireScreen(t, m, "/ web")
			}
		})
	}
}

func TestTailPromptNamesTheRequestTarget(t *testing.T) {
	t.Parallel()

	target := views.ResourceRef{Kind: "deployment", Namespace: tuitest.NamespaceDemo, Name: tuitest.DeploymentWeb}
	req := views.TailPromptRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Target: target}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	requireScreen(t, m, "tail lines · deployment/web · ns demo", "❯ 100")

	_, out := tuitest.Drive(t, m, tuitest.Enter())
	want := views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Tail: 100}
	if !tuitest.Contains(out, want) {
		t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
}

func TestContainerPickerCarriesTheTargetOn(t *testing.T) {
	t.Parallel()

	target := views.ResourceRef{Kind: "deployment", Namespace: tuitest.NamespaceDemo, Name: tuitest.DeploymentWeb}
	req := containerRequest("logs", tuitest.PodWeb1, tuitest.PodWeb2)
	req.Target = target
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	requireScreen(t, m, "container · deployment/web · ns demo")

	m, out := tuitest.Drive(t, m, tuitest.Press("2"))
	want := views.TailPromptRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2},
		Container: tuitest.ContainerSidecar, Target: target,
	}
	if !tuitest.Contains(out, want) {
		t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	requireScreen(t, m, "tail lines · deployment/web · ns demo")
}

// The command bar takes the footer's first row and leaves the body and the
// kubectl row where they were.
func TestCommandBarSitsInTheFooter(t *testing.T) {
	t.Parallel()

	m := started(t, tuitest.NewFakeClient(t))
	before := lines(m)
	m, _ = tuitest.Drive(t, m, tuitest.Press(":"))
	after := lines(m)

	block := footerBlock(t, m)
	if got := strings.TrimSpace(block[1]); got != ": pods, ns, dashboard" {
		t.Errorf("first footer row = %q", got)
	}
	if got := strings.TrimSpace(block[2]); got != "kubectl get all -A" {
		t.Errorf("command row = %q", got)
	}
	body := len(after) - len(block)
	if strings.Join(before[:body], "\n") != strings.Join(after[:body], "\n") {
		t.Errorf("opening the command bar moved the body:\n%s", tuitest.Screen(m))
	}
}

func TestPasteGoesIntoTextPromptsOnly(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input []tea.Msg
		want  int64 // tail of the logs request; 0: none
	}{
		{name: "tail prompt", input: []tea.Msg{tailRequest(), clearInput(), tuitest.Paste("42"), tuitest.Enter()}, want: 42},
		{name: "container picker ignores it", input: []tea.Msg{containerRequest("logs", tuitest.PodMulti), tuitest.Paste("2")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.input...)
			logs := tuitest.All[views.LogsRequestMsg](out)
			if tc.want == 0 {
				if len(logs) != 0 || len(tuitest.All[views.TailPromptRequestMsg](out)) != 0 {
					t.Fatalf("paste acted:\n%s", tuitest.Describe(out))
				}
				requireScreen(t, m, "▶ 1. app")
				return
			}
			if len(logs) != 1 || logs[0].Tail != tc.want {
				t.Fatalf("logs requests = %+v, want tail %d", logs, tc.want)
			}
		})
	}
}
