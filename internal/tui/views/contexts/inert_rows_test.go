package contexts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/contexts"
)

// hostileProd is the raw name of the one hostile context among clean
// ones. It sorts after them (U+202E is past ASCII), so it is never row 0.
var hostileProd = tuitest.Hostile("prod")

// writeContexts writes path as a JSON kubeconfig with one context, cluster
// and user per name, so every byte of a name survives.
func writeContexts(t *testing.T, path string, names ...string) {
	t.Helper()
	clusters := make([]any, 0, len(names))
	users := make([]any, 0, len(names))
	ctxs := make([]any, 0, len(names))
	for _, n := range names {
		clusters = append(clusters, map[string]any{"name": n + "-c", "cluster": map[string]any{"server": tuitest.UnreachableServer}})
		users = append(users, map[string]any{"name": n + "-u", "user": map[string]any{"token": "placeholder"}})
		ctxs = append(ctxs, map[string]any{"name": n, "context": map[string]any{"cluster": n + "-c", "user": n + "-u"}})
	}
	body, err := json.Marshal(map[string]any{
		"apiVersion": "v1", "kind": "Config", "current-context": names[0],
		"clusters": clusters, "users": users, "contexts": ctxs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func mixedContexts(t *testing.T) (string, contexts.Model) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	writeContexts(t, path, "alpha", "beta", hostileProd)
	m, _ := tuitest.Mount(t, contexts.New("alpha", k8s.Source{Paths: []string{path}, Origin: k8s.OriginFlag}), 120, 20)
	return path, m
}

// Enter switches to the raw name of the context under the cursor.
func TestEnterSwitchesToTheRawContextUnderTheCursor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []string
		want string
	}{
		{name: "first row", want: "alpha"},
		{name: "a clean row before the hostile one", keys: []string{"down"}, want: "beta"},
		{name: "the hostile row", keys: []string{"down", "down"}, want: hostileProd},
		{name: "back up from the hostile row", keys: []string{"down", "down", "up"}, want: "beta"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, m := mixedContexts(t)
			m2, _ := tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			if got := m2.State().Selected; got != views.RowKey("", tc.want) {
				t.Errorf("State().Selected = %q, want %q", got, tc.want)
			}
			_, msgs := tuitest.Drive(t, m2, tuitest.Enter())
			if want := (views.ContextSelectedMsg{Name: tc.want}); !tuitest.Contains(msgs, want) {
				t.Errorf("enter sent %v, want %+v", msgs, want)
			}
		})
	}
}

// r rereads the kubeconfig; a context added above keeps the cursor on the
// same raw context, so Enter does not switch to another one.
func TestRefreshKeepsTheCursorOnTheRawContext(t *testing.T) {
	t.Parallel()

	path, m := mixedContexts(t)
	m2, _ := tuitest.Drive(t, m, tuitest.Keys("down", "down")...)
	writeContexts(t, path, "alpha", "aaa", "beta", hostileProd)
	m2, _ = tuitest.Drive(t, m2, tuitest.Press("r"))
	if st := m2.State(); st.Selected != views.RowKey("", hostileProd) || st.Cursor != 3 {
		t.Fatalf("after r the cursor is on %q at %d, want the hostile context at 3", st.Selected, st.Cursor)
	}
	_, msgs := tuitest.Drive(t, m2, tuitest.Enter())
	if want := (views.ContextSelectedMsg{Name: hostileProd}); !tuitest.Contains(msgs, want) {
		t.Errorf("enter sent %v, want %+v", msgs, want)
	}
}
