package contexts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
)

// hostileKubeconfig writes a kubeconfig, as JSON so every byte survives,
// whose one context has a name, cluster, user and namespace carrying
// escape sequences and invisible characters, and returns the context's
// raw name and the file.
func hostileKubeconfig(t *testing.T) (string, k8s.Source) {
	t.Helper()
	name := tuitest.Hostile("prod")
	cluster, user := tuitest.Hostile("eks-prod"), tuitest.Hostile("admin")
	cfg := map[string]any{
		"apiVersion":      "v1",
		"kind":            "Config",
		"current-context": name,
		"clusters":        []any{map[string]any{"name": cluster, "cluster": map[string]any{"server": tuitest.UnreachableServer}}},
		"users":           []any{map[string]any{"name": user, "user": map[string]any{"token": "placeholder"}}},
		"contexts": []any{map[string]any{"name": name, "context": map[string]any{
			"cluster": cluster, "user": user, "namespace": tuitest.Hostile("shop"),
		}}},
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return name, k8s.Source{Paths: []string{path}, Origin: k8s.OriginFlag}
}

// The row shows the cleaned names; Enter switches to the raw context
// name, read from the kubeconfig and never back from a cell. A kubeconfig
// name is free text, so a cleaned one could name another context.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current func(raw string) string
	}{
		{name: "the current context", current: func(raw string) string { return raw }},
		{name: "another context", current: func(string) string { return "dev" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, src := hostileKubeconfig(t)
			m, _ := tuitest.Mount(t, contexts.New(tc.current(raw), src), 120, 20)

			tuitest.AssertInert(t, tuitest.Frame(m))
			screen := tuitest.Screen(m)
			for _, w := range []string{"prod", "eks-prod", "admin", "shop"} {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen)
				}
			}
			if got := m.State().Selected; got != views.RowKey("", raw) {
				t.Errorf("State().Selected = %q, want the raw row key", got)
			}
			_, msgs := tuitest.Drive(t, m, tuitest.Enter())
			if want := (views.ContextSelectedMsg{Name: raw}); !tuitest.Contains(msgs, want) {
				t.Errorf("enter sent %v, want the raw %+v", msgs, want)
			}
		})
	}
}
