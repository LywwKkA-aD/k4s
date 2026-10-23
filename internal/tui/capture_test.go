package tui_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// While a view's own prompt is focused, the root forwards every key to it:
// q, :, ? and Esc are text or prompt control, not navigation.

func TestFilterPromptReceivesGlobalKeys(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tuitest.Keys("/"))...)
	m, out := tuitest.Drive(t, m, tuitest.Press("q"), tuitest.Press("?"), tuitest.Press(":"))
	if hasQuit(out) {
		t.Fatal("q in the filter quit")
	}
	requireView(t, m, "pods")
	requireScreen(t, m, "/ q?:")
	refuteScreen(t, m, "force quit")
	if strings.HasPrefix(strings.TrimSpace(footer(m)), ": ") {
		t.Fatal(": in the filter opened the command bar")
	}

	// Esc closes the filter instead of going back.
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
	refuteScreen(t, m, "/ q?:")

	// With the prompt gone, q is navigation again.
	m, _ = tuitest.Drive(t, m, tuitest.Press("q"))
	requireView(t, m, "dashboard")
}

func TestLogsSearchReceivesGlobalKeys(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), colon("pods")...)
	m, _ = tuitest.Drive(t, m, views.LogsRequestMsg{Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}, Tail: 10})
	requireView(t, m, "pod · web-1 · logs")

	m, out := tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Type("fake"), tuitest.Press("q"))
	if hasQuit(out) {
		t.Fatal("q in the search prompt quit")
	}
	requireView(t, m, "pod · web-1 · logs")
	requireScreen(t, m, "/fakeq")

	m, _ = tuitest.Drive(t, m, tuitest.Press("backspace"), tuitest.Enter())
	requireView(t, m, "pod · web-1 · logs · /fake · 1/1 · paused")

	// Esc with no prompt open goes back again.
	m, _ = tuitest.Drive(t, m, tuitest.Esc())
	requireView(t, m, "pods")
}
