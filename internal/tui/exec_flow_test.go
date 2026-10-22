package tui_test

// These tests point PATH at a temporary directory with t.Setenv, so they
// cannot run in parallel.

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// pathWithKubectl puts a do-nothing kubectl on PATH. The driver never runs
// it: the exec hand-off is only recorded.
func pathWithKubectl(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stand-in kubectl is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { //nolint:gosec // the stand-in must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

func TestExecFromThePodsView(t *testing.T) {
	pathWithKubectl(t)

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo})
	m, out := tuitest.Drive(t, m, then(tuitest.FilterBy(tuitest.PodWeb1), tuitest.Keys("e"))...)
	want := views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1, Container: tuitest.ContainerNginx, Shell: views.ExecShell}
	if !tuitest.Contains(out, want) {
		t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	if !slices.ContainsFunc(out, tuitest.IsExec) {
		t.Errorf("the root did not hand the terminal to kubectl:\n%s", tuitest.Describe(out))
	}
	if got := footer(m); strings.Contains(got, "exec:") {
		t.Errorf("footer = %q", got)
	}
}

// TestExecInAWindowsPodFromThePodsView: the pods view's lookup names
// cmd.exe and the root runs it without looking the pod up again.
func TestExecInAWindowsPodFromThePodsView(t *testing.T) {
	pathWithKubectl(t)

	win := tuitest.Pod(tuitest.NamespaceDemo, "iis-0", nil, "web")
	win.Spec.OS = &corev1.PodOS{Name: corev1.Windows}
	var gets atomic.Int32
	count := tuitest.WithReactor("get", "pods", func(a k8stesting.Action) (bool, kruntime.Object, error) {
		if a.GetSubresource() == "" {
			gets.Add(1)
		}
		return false, nil, nil
	})
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(tuitest.Namespace(tuitest.NamespaceDemo), win), count)
	m, _ := tuitest.Drive(t, started(t, c), views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDemo})
	_, out := tuitest.Drive(t, m, then(tuitest.FilterBy("iis-0"), tuitest.Keys("e"))...)
	want := views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: "iis-0", Container: "web", Shell: views.WindowsShell}
	if !tuitest.Contains(out, want) {
		t.Fatalf("want %+v, got:\n%s", want, tuitest.Describe(out))
	}
	if !slices.ContainsFunc(out, tuitest.IsExec) {
		t.Errorf("the root did not hand the terminal to kubectl:\n%s", tuitest.Describe(out))
	}
	if n := gets.Load(); n != 1 {
		t.Errorf("the pod was read %d times, want once, by the pods view", n)
	}
}

func TestExecWithoutKubectlWarnsInTheFooter(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	m, out := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)),
		views.ExecRequestMsg{Namespace: tuitest.NamespaceDemo, Pod: tuitest.PodWeb1})
	if slices.ContainsFunc(out, tuitest.IsExec) {
		t.Fatal("exec started without kubectl")
	}
	if got := footer(m); !strings.Contains(got, "exec: kubectl not found on PATH") {
		t.Errorf("footer = %q", got)
	}
}
