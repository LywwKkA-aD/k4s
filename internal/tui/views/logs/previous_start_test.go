package logs_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

// TestWithPreviousOpensThePreviousInstance: a view asked for the previous
// instance (the dashboard's "why does it keep crashing?") opens on it, as
// if p had been pressed: p goes back to the current one.
func TestWithPreviousOpensThePreviousInstance(t *testing.T) {
	t.Parallel()

	in := &instances{current: "serving\n", previous: "panic: boom\n"}
	c := tuitest.NewFakeClient(t, in.option())
	m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 100, tuitest.ContainerNginx, logs.WithPrevious()), 100, 20)
	t.Cleanup(func() { _ = m.Close() })
	requireTitle(t, m, "pod · web-1 / nginx · logs · previous")
	if got := m.Command().String(); got != "kubectl logs web-1 --tail=100 -n demo -c nginx --previous" {
		t.Errorf("command = %q", got)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, "panic: boom") || strings.Contains(screen, "serving") {
		t.Errorf("want the previous instance only:\n%s", screen)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("p"))
	requireTitle(t, m, "pod · web-1 / nginx · logs")
}

// TestWithPreviousWithoutOneKeepsTheCurrentLog: a container that never
// restarted keeps the current log and says why, as p does.
func TestWithPreviousWithoutOneKeepsTheCurrentLog(t *testing.T) {
	t.Parallel()

	in := &instances{current: "serving\n", prevErr: noPrevious(tuitest.ContainerNginx, tuitest.PodWeb1)}
	c := tuitest.NewFakeClient(t, in.option())
	m, out := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 100, tuitest.ContainerNginx, logs.WithPrevious()), 100, 20)
	t.Cleanup(func() { _ = m.Close() })
	got, ok := tuitest.Find[views.StatusMsg](out)
	if !ok || got.Level != status.Warn || !strings.Contains(got.Text, "has no previous instance") {
		t.Fatalf("status = %+v (found %v), want the no-previous warning", got, ok)
	}
	if strings.Contains(m.Title(), "previous") || !strings.Contains(tuitest.Screen(m), "serving") {
		t.Errorf("want the current log:\ntitle %q\n%s", m.Title(), tuitest.Screen(m))
	}
}
