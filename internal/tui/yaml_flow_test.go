package tui_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// kubectlRow is the footer's command row.
func kubectlRow(m interface{ View() tea.View }) string {
	rows := strings.Split(tuitest.Footer(m), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// TestOOpensTheSelectedObjectsYAML: o on a list opens the YAML of the row
// under the cursor, with `kubectl get ... -o yaml` in the footer.
func TestOOpensTheSelectedObjectsYAML(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		keys  []tea.Msg
		view  string
		cmd   string
		lines []string
	}{
		{
			name: "pods", keys: then(colon("pods"), tuitest.Keys("down", "o")),
			view: "pod · multi-0 · yaml", cmd: "kubectl get pod multi-0 -n demo -o yaml",
			lines: []string{"kind: Pod", "name: multi-0"},
		},
		{
			name: "deployments", keys: then(colon("deploy"), tuitest.Keys("o")),
			view: "deployment · multi · yaml", cmd: "kubectl get deployment multi -n demo -o yaml",
			lines: []string{"kind: Deployment"},
		},
		{
			name: "services", keys: then(colon("svc"), tuitest.Keys("down", "o")),
			view: "service · web · yaml", cmd: "kubectl get service web -n demo -o yaml",
			lines: []string{"kind: Service"},
		},
		{
			name: "namespaces", keys: then(colon("ns"), tuitest.Keys("down", "down", "o")),
			view: "namespace · demo · yaml", cmd: "kubectl get namespace demo -o yaml",
			lines: []string{"kind: Namespace"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.keys...)
			if got := viewName(m); !strings.HasPrefix(got, tc.view) {
				t.Fatalf("view = %q, want %q\n%s", got, tc.view, tuitest.Screen(m))
			}
			requireScreen(t, m, append([]string{"apiVersion:"}, tc.lines...)...)
			if got := kubectlRow(m); got != tc.cmd {
				t.Errorf("footer command = %q, want %q", got, tc.cmd)
			}
		})
	}
}

// TestEscFromYAMLRestoresTheList: the list comes back as it was left, its
// filter and its row (S17).
func TestEscFromYAMLRestoresTheList(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.FilterBy("web"), tuitest.Keys("down", "o"))...)
	if got := viewName(m); !strings.HasPrefix(got, "pod · web-2 · yaml") {
		t.Fatalf("view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
	requireScreen(t, m, "/ web")
	m, _ = tuitest.Drive(t, m, tuitest.Press("o"))
	if got := viewName(m); !strings.HasPrefix(got, "pod · web-2 · yaml") {
		t.Errorf("the cursor did not come back to web-2: view = %q", got)
	}
}

// TestOFromDescribeAndBack: describe → o → YAML of the same object; Esc
// returns to describe, then to the list.
func TestOFromDescribeAndBack(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("deploy"), tuitest.Keys("down", "enter"))...)
	if got := viewName(m); !strings.HasPrefix(got, "deployment · web · built-in") {
		t.Fatalf("view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("o"))
	if got := viewName(m); !strings.HasPrefix(got, "deployment · web · yaml") {
		t.Fatalf("o in describe: view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	if got := viewName(m); !strings.HasPrefix(got, "deployment · web · built-in") {
		t.Fatalf("Esc from YAML: view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "deployments")
}

// TestOAgainInYAMLReloadsInPlace: o in the YAML view fetches it again and
// does not stack a second copy under Esc.
func TestOAgainInYAMLReloadsInPlace(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys("o", "o", "o"))...)
	if got := viewName(m); !strings.HasPrefix(got, "pod · crash-0 · yaml") {
		t.Fatalf("view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
}

// TestOWithoutAnObjectSaysSo: a view without a row answers o with a
// status, never silence.
func TestOWithoutAnObjectSaysSo(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		client bool
		keys   []tea.Msg
		view   string
		status string
	}{
		{name: "the dashboard's banner", client: true, keys: tuitest.Keys("o"), view: "dashboard", status: "yaml: no row selected"},
		{name: "the <all> namespace row", client: true, keys: then(colon("ns"), tuitest.Keys("o")), view: "namespaces", status: "yaml: no row selected"},
		{name: "an empty filter", client: true, keys: then(colon("pods"), tuitest.FilterBy("zzz"), tuitest.Keys("o")), view: "pods", status: "yaml: no row selected"},
		{name: "no kubeconfig", keys: then(colon("pods"), tuitest.Keys("o")), view: "pods", status: "no kubeconfig"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			start := started(t, nil)
			if tc.client {
				start = started(t, tuitest.NewFakeClient(t))
			}
			m, _ := tuitest.Drive(t, start, tc.keys...)
			requireView(t, m, tc.view)
			if got := footer(m); !strings.Contains(got, tc.status) {
				t.Errorf("footer = %q, want %q", got, tc.status)
			}
		})
	}
}

// TestOIsTextInAPrompt: while a filter is being typed, o is a letter.
func TestOIsTextInAPrompt(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys("/", "o"))...)
	requireView(t, m, "pods")
	requireScreen(t, m, "/ o")
}

// TestYCopiesTheYAMLCommand: y in the YAML view copies the footer's
// command.
func TestYCopiesTheYAMLCommand(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys("o", "y"))...)
	if got := footer(m); !strings.Contains(got, "copied: kubectl get pod crash-0 -n demo -o yaml") {
		t.Errorf("footer = %q", got)
	}
}

// TestYAMLRequestMsgOpensTheView: another part of k4s can ask for an
// object's YAML with the message alone.
func TestYAMLRequestMsgOpensTheView(t *testing.T) {
	t.Parallel()

	ref := views.ResourceRef{Kind: "service", Namespace: tuitest.NamespaceDemo, Name: tuitest.ServiceWeb}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.YAMLRequestMsg{Ref: ref})
	if got := viewName(m); !strings.HasPrefix(got, "service · web · yaml") {
		t.Fatalf("view = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "dashboard")
}
