package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

var subprocess = tuitest.Driver{CmdTimeout: 15 * time.Second}

// TestDescribeRunsTheFooterCommand: describe runs kubectl with exactly the
// command the footer shows, --kubeconfig included, and the screen shows
// what kubectl printed.
func TestDescribeRunsTheFooterCommand(t *testing.T) {
	t.Parallel()

	cfg, argvFile := fakeKubectl(t)
	c := tuitest.NewFakeClient(t)
	// Neither the file nor the context a bare kubectl would use here, so
	// the footer names both.
	c.KubeconfigPaths = []string{filepath.FromSlash("/k4s-test/cluster.yaml")}
	c.CurrentContext = "elsewhere"
	m, _ := tuitest.Mount(t, New(c, nil, WithRunnerConfig(cfg), instantFrames()), 160, 40)

	req := views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}
	m, _ = tuitest.DriveWith(t, subprocess, m, req)

	rows := strings.Split(tuitest.Footer(m), "\n")
	shown := strings.TrimSpace(rows[len(rows)-1])
	ran := "kubectl " + strings.Join(ranArgv(t, argvFile), " ")
	if ran != shown {
		t.Errorf("kubectl ran  %q\nfooter shows %q", ran, shown)
	}
	if !strings.Contains(shown, "--kubeconfig "+c.KubeconfigPaths[0]+" --context "+c.Context+" describe") {
		t.Errorf("the footer does not pin the kubeconfig and context: %q", shown)
	}
	if screen := tuitest.Screen(m); !strings.Contains(screen, fakeDescribed) || !strings.Contains(screen, "via kubectl v1.33.9") {
		t.Errorf("screen:\n%s", screen)
	}
}

// TestDescribeWithoutKubeconfigFilesIsBuiltIn: a client that names no
// kubeconfig files cannot point kubectl at its cluster, so the built-in
// describer answers and kubectl never runs.
func TestDescribeWithoutKubeconfigFilesIsBuiltIn(t *testing.T) {
	t.Parallel()

	cfg, argvFile := fakeKubectl(t)
	m, _ := tuitest.Mount(t, New(tuitest.NewFakeClient(t), nil, WithRunnerConfig(cfg), instantFrames()), 160, 40)
	req := views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}
	m, _ = tuitest.DriveWith(t, subprocess, m, req)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "built-in") || strings.Contains(screen, fakeDescribed) {
		t.Errorf("screen:\n%s", screen)
	}
	if _, err := os.Stat(argvFile); err == nil {
		t.Error("kubectl ran for a client without kubeconfig files")
	}
}
