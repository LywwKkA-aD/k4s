package tui_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestRecapGolden walks one session through view opens, describe, logs and
// exec, then prints the exit recap. Regenerate with
//
//	go test ./internal/tui -run TestRecapGolden -update
//
// It points PATH at an empty directory, so it does not run in parallel.
func TestRecapGolden(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	demo := tuitest.NamespaceDemo
	m := started(t, tuitest.NewFakeClient(t))
	m, _ = tuitest.Drive(t, m, colon("pods")...)
	m, _ = tuitest.Drive(t, m, views.DescribeRequestMsg{Kind: "pod", Namespace: demo, Name: tuitest.PodWeb1}, tuitest.Esc())
	m, _ = tuitest.Drive(t, m, views.LogsRequestMsg{Namespace: demo, Pods: []string{tuitest.PodWeb1}, Tail: 100}, tuitest.Esc())
	m, _ = tuitest.Drive(t, m, views.ExecRequestMsg{Namespace: demo, Pod: tuitest.PodWeb1, Container: tuitest.ContainerNginx})
	m, _ = tuitest.Drive(t, m, colon("deploy")...)
	m, _ = tuitest.Drive(t, m, colon("pods")...)

	var out bytes.Buffer
	if err := tui.WriteRecap(&out, m.Recap()); err != nil {
		t.Fatalf("WriteRecap: %v", err)
	}
	tuitest.Golden(t, "recap", out.String())
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestWriteRecap(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	if err := tui.WriteRecap(&out, nil); err != nil || out.Len() != 0 {
		t.Fatalf("an empty recap printed %q, err %v", out.String(), err)
	}
	if err := tui.WriteRecap(brokenWriter{}, []string{"kubectl get pods"}); err == nil {
		t.Fatal("a failed write went unreported")
	}
}

// A fresh session has shown no command yet: the landing dashboard is not a
// command the user asked for.
func TestRecapStartsEmpty(t *testing.T) {
	t.Parallel()

	if got := started(t, tuitest.NewFakeClient(t)).Recap(); len(got) != 0 {
		t.Fatalf("recap = %q, want none", got)
	}
}
