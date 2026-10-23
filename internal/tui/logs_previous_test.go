package tui_test

import (
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestLogsRequestForThePreviousInstance: a request with Previous (the
// dashboard's crash-loop suggestion) opens the logs view on the previous
// instance of the container, as p in that view does, without the tail
// prompt.
func TestLogsRequestForThePreviousInstance(t *testing.T) {
	t.Parallel()

	req := views.LogsRequestMsg{
		Namespace: tuitest.NamespaceDemo, Pods: []string{tuitest.PodWeb1}, Container: tuitest.ContainerNginx,
		Target: views.ResourceRef{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}, Previous: true,
	}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req)
	requireView(t, m, "pod · web-1 / nginx · logs · previous")
	if got, want := commandRow(m), "kubectl logs web-1 --tail=100 -n demo -c nginx --previous"; got != want {
		t.Fatalf("command row = %q, want %q", got, want)
	}
}
