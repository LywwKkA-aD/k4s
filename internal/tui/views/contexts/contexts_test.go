package contexts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
)

const (
	alpha = "alpha"
	beta  = "beta"
)

func mounted(t *testing.T, current string) contexts.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, contexts.New(current, k8s.Source{}), 120, 20)
	return m
}

func TestListsContextsSortedWithCurrentMarker(t *testing.T) {
	t.Parallel()

	screen := tuitest.Screen(mounted(t, beta))
	for _, w := range []string{"NAME", "CLUSTER", "USER", "alpha-cluster", "beta-user", tuitest.NamespaceDemo} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	a, b := strings.Index(screen, alpha), strings.Index(screen, beta)
	if a < 0 || b < 0 || a > b {
		t.Errorf("rows are not sorted by name:\n%s", screen)
	}
	for _, line := range strings.Split(screen, "\n") {
		if strings.Contains(line, "★") != strings.Contains(line, "beta-cluster") {
			t.Errorf("only the current context carries the marker: %q", line)
		}
	}
}

func TestEnterSelectsContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []string
		want string
	}{
		{name: "first row", keys: []string{"enter"}, want: alpha},
		{name: "second row", keys: []string{"down", "enter"}, want: beta},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, out := tuitest.Drive(t, mounted(t, alpha), tuitest.Keys(tc.keys...)...)
			want := views.ContextSelectedMsg{Name: tc.want}
			if got := tuitest.Requests(out); len(got) != 1 || !tuitest.Contains(got, want) {
				t.Errorf("want only %+v, got:\n%s", want, tuitest.Describe(out))
			}
		})
	}
}

func TestEnterBeforeLoadDoesNothing(t *testing.T) {
	t.Parallel()

	m := contexts.New(alpha, k8s.Source{})
	if !strings.Contains(tuitest.Screen(m), "loading kubeconfig") {
		t.Errorf("before the read: %q", tuitest.Screen(m))
	}
	if _, out := tuitest.Drive(t, m, tuitest.Enter()); len(out) != 0 {
		t.Errorf("enter before load emitted:\n%s", tuitest.Describe(out))
	}
}

func TestRefreshRereadsKubeconfig(t *testing.T) {
	t.Parallel()

	m := mounted(t, alpha)
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	if len(out) != 1 {
		t.Errorf("r should reread once:\n%s", tuitest.Describe(out))
	}
}

func TestStaticContract(t *testing.T) {
	t.Parallel()

	m := contexts.New("", k8s.Source{})
	if m.Title() != "contexts" || m.Command().String() != "kubectl config get-contexts" {
		t.Errorf("Title=%q Command=%q", m.Title(), m.Command().String())
	}
	if m.CapturesKeys() || m.Close() != nil || len(m.Help()) != 2 {
		t.Error("contexts captures no keys, owns nothing and offers enter and r")
	}
}

// TestKubeconfigProblems changes KUBECONFIG, so it cannot run in parallel.
func TestKubeconfigProblems(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "broken")
	if err := os.WriteFile(broken, []byte("apiVersion: v1\nkind: Config\ncontexts: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		path string
		want string
	}{
		{name: "no file", path: filepath.Join(dir, "missing"), want: "no contexts in kubeconfig"},
		{name: "unparseable file", path: broken, want: "contexts unavailable:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KUBECONFIG", tc.path)
			if screen := tuitest.Screen(mounted(t, "")); !strings.Contains(screen, tc.want) {
				t.Errorf("screen = %q, want %q", screen, tc.want)
			}
		})
	}
}

// TestListsTheSessionKubeconfig: the picker reads the files the session
// loaded (--kubeconfig, the k3s fallback), not whatever KUBECONFIG says.
func TestListsTheSessionKubeconfig(t *testing.T) {
	t.Parallel()

	path, err := tuitest.WriteKubeconfig(t.TempDir(), "gamma", "gamma", "delta")
	if err != nil {
		t.Fatal(err)
	}
	src := k8s.Source{Paths: []string{path}, Origin: k8s.OriginFlag}
	m, _ := tuitest.Mount(t, contexts.New("gamma", src), 120, 20)
	screen := tuitest.Screen(m)
	for _, w := range []string{"gamma-cluster", "delta-cluster"} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	if strings.Contains(screen, "alpha-cluster") {
		t.Errorf("the picker read KUBECONFIG instead of the session's file:\n%s", screen)
	}

	missing := k8s.Source{Paths: []string{path + ".gone"}, Origin: k8s.OriginFlag}
	m, _ = tuitest.Mount(t, contexts.New("", missing), 120, 20)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "contexts unavailable:") {
		t.Errorf("a missing session file: %q", screen)
	}
}
