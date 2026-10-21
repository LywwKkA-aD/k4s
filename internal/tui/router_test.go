package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/command"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func TestEveryCommandAliasSwitchesView(t *testing.T) {
	t.Parallel()

	for _, name := range command.All() {
		aliases := []string{name}
		if extra := command.Aliases(name); extra != "" {
			aliases = append(aliases, strings.Split(extra, ", ")...)
		}
		for _, alias := range aliases {
			t.Run(alias, func(t *testing.T) {
				t.Parallel()
				m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon(alias)...)
				requireView(t, m, name)
				if got := strings.Count(tuitest.Screen(m), "\n") + 1; got != screenH {
					t.Errorf("screen is %d lines after the switch, want %d", got, screenH)
				}
			})
		}
	}
}

func TestCommandInputIsTrimmedAndCaseInsensitive(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, nil), colon("  SVC ")...)
	requireView(t, m, "services")
}

func TestUnknownCommandShowsFooterError(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, nil), colon("nope")...)
	requireView(t, m, "dashboard")
	if got := footer(m); !strings.Contains(got, "unknown command: nope") {
		t.Errorf("footer = %q", got)
	}

	// The next command bar starts clean.
	m, _ = tuitest.Drive(t, m, tuitest.Press(":"))
	if got := strings.TrimSpace(footer(m)); strings.Contains(got, "unknown command") || !strings.HasPrefix(got, ": ") {
		t.Errorf("command bar footer = %q", got)
	}
}

func TestCommandBarEscCancels(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, nil), tuitest.Press(":"), tuitest.Type("pods"), tuitest.Esc())
	requireView(t, m, "dashboard")
	if got := footer(m); !strings.Contains(got, "q quit") {
		t.Errorf("footer after esc = %q", got)
	}
}

func TestEscWalksHistoryBack(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("ns"), colon("pods"), colon("pods"))...)
	requireView(t, m, "pods")

	for _, want := range []string{"namespaces", "dashboard", "dashboard"} {
		m, _ = tuitest.Drive(t, m, tuitest.Esc())
		requireView(t, m, want)
	}
}

func TestNamespaceSelectionScopesTheNextViews(t *testing.T) {
	t.Parallel()

	m, out := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("ns"), tuitest.Keys("down", "down", "enter"))...)
	if !tuitest.Contains(out, views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo}) {
		t.Fatalf("namespace pick not emitted:\n%s", tuitest.Describe(out))
	}
	requireView(t, m, "pods")
	if !strings.Contains(header(m), "ns demo") {
		t.Errorf("header = %q", header(m))
	}
	if !strings.HasSuffix(footer(m), "\n kubectl get pods -n demo") {
		t.Errorf("footer = %q", footer(m))
	}
	refuteScreen(t, m, tuitest.PodCoreDNS)

	m, _ = tuitest.Drive(t, m, colon("deploy")...)
	if !strings.HasSuffix(footer(m), "\n kubectl get deployments -n demo") {
		t.Errorf("deployments footer = %q", footer(m))
	}

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "namespaces")
	if !strings.Contains(header(m), "ns ALL") {
		t.Errorf("history restores the namespace in effect: %q", header(m))
	}
}

func TestQuitGoesHomeThenQuits(t *testing.T) {
	t.Parallel()

	m, out := tuitest.Drive(t, started(t, nil), then(colon("svc"), colon("pods"), tuitest.Keys("q"))...)
	requireView(t, m, "dashboard")
	if hasQuit(out) {
		t.Fatal("q away from the dashboard must go home, not quit")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "dashboard")

	_, out = tuitest.Drive(t, m, tuitest.Press("q"))
	if !hasQuit(out) {
		t.Errorf("q on the dashboard must quit:\n%s", tuitest.Describe(out))
	}
}

func TestCtrlCQuitsFromAnywhere(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup []string // commands or keys, see below
		keys  []string
	}{
		{name: "dashboard"},
		{name: "list view", setup: []string{"pods"}},
		{name: "command bar", keys: []string{":"}},
		{name: "help popup", keys: []string{"?"}},
		{name: "filter prompt", setup: []string{"pods"}, keys: []string{"/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := started(t, tuitest.NewFakeClient(t))
			for _, c := range tc.setup {
				m, _ = tuitest.Drive(t, m, colon(c)...)
			}
			m, _ = tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			if _, out := tuitest.Drive(t, m, tuitest.Press("ctrl+c")); !hasQuit(out) {
				t.Errorf("ctrl+c did not quit:\n%s", tuitest.Describe(out))
			}
		})
	}
}

func TestLeafViewsAreSkippedByHistory(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon("pods")...)
	m, _ = tuitest.Drive(t, m, views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	requireView(t, m, "pod · web-1 · built-in")
	requireScreen(t, m, "Name:", tuitest.PodWeb1)
	if !strings.Contains(footer(m), "kubectl describe pod web-1 -n demo") {
		t.Errorf("describe footer = %q", footer(m))
	}

	m, _ = tuitest.Drive(t, m, views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}, Tail: 50})
	requireView(t, m, "logs · web-1")
	requireScreen(t, m, "fake logs")

	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "dashboard")
}

func TestScreenKeepsTerminalSizeAcrossResizes(t *testing.T) {
	t.Parallel()

	m := started(t, tuitest.NewFakeClient(t))
	sizes := []struct{ w, h int }{{80, 24}, {120, 30}, {200, 60}}
	for _, s := range sizes {
		m, _ = tuitest.Drive(t, m, tuitest.Resize(s.w, s.h))
		for _, c := range []string{"pods", "top", "dashboard"} {
			m, _ = tuitest.Drive(t, m, colon(c)...)
			if got := strings.Count(tuitest.Screen(m), "\n") + 1; got != s.h {
				t.Errorf("%dx%d on %s: %d lines", s.w, s.h, c, got)
			}
		}
	}
}

func TestHeaderShowsContextAndNamespace(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		with bool
		want []string
		not  []string
	}{
		{name: "with a client", with: true, want: []string{"k4s", "ctx " + tuitest.FakeContext, "ns ALL", " · dashboard"}},
		{name: "without a client", want: []string{"k4s", "ns ALL", " · dashboard"}, not: []string{"ctx "}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := started(t, nil)
			if tc.with {
				m = started(t, tuitest.NewFakeClient(t))
			}
			h := header(m)
			for _, w := range tc.want {
				if !strings.Contains(h, w) {
					t.Errorf("header %q lacks %q", h, w)
				}
			}
			for _, w := range tc.not {
				if strings.Contains(h, w) {
					t.Errorf("header %q shows %q", h, w)
				}
			}
		})
	}
}
