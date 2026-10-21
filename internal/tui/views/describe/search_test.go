package describe_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

func loadedPod(t *testing.T, w, h int) describe.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, describe.New(tuitest.NewFakeClient(t), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodMulti), w, h)
	return m
}

// TestSearchInDescribe: "/" searches the describe text; while the prompt
// is open the view takes every key, and Esc clears a committed search
// before it means back.
func TestSearchInDescribe(t *testing.T) {
	t.Parallel()

	m := loadedPod(t, 100, 12)
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"))
	if !m.CapturesKeys() {
		t.Fatal("the open search prompt does not take the keys")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Type("sidecar"), tuitest.Enter())
	if m.CapturesKeys() {
		t.Fatal("the prompt stayed open after Enter")
	}
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "/ sidecar") || !strings.Contains(screen, "1/") {
		t.Errorf("no search bar with a match count:\n%s", screen)
	}

	next, _, used := m.Back()
	if !used {
		t.Fatal("Esc with a search on went back instead of clearing it")
	}
	m = next.(describe.Model)
	if strings.Contains(tuitest.Screen(m), "/ sidecar") {
		t.Errorf("the search survived Esc:\n%s", tuitest.Screen(m))
	}
	if _, _, used := m.Back(); used {
		t.Error("Esc without a search did not go back")
	}
}

func TestDescribeSelectsItsObject(t *testing.T) {
	t.Parallel()

	var sel views.Selection = describe.New(nil, describe.KindDeployment, "shop", "api")
	ref, ok := sel.Selected()
	want := views.ResourceRef{Kind: "deployment", Namespace: "shop", Name: "api"}
	if !ok || ref != want {
		t.Errorf("Selected = %+v, %v; want %+v", ref, ok, want)
	}
}
