package tui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const fakeKubectlPath = "/usr/local/bin/kubectl"

// withLookPath gives m a runner that finds kubectl through look and runs
// with base as its environment.
func withLookPath(m Model, look func(string) (string, error), base []string) Model {
	m.runner = kubectl.NewRunner(kubectl.RunnerConfig{
		Kubeconfig: kubeconfigPaths(m.client),
		LookPath:   look,
		Environ:    func() []string { return slices.Clone(base) },
	})
	return m
}

func foundAt(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func notFound(file string) (string, error) {
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

func kubeconfigEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, kubectl.EnvKubeconfig+"=") {
			out = append(out, kv)
		}
	}
	return out
}

// TestExecRunsTheFooterCommand is the exec contract: the decorated command
// the footer shows, pasted into a shell, splits into exactly the argv k4s
// runs, and the child reads the same kubeconfig files as the TUI.
func TestExecRunsTheFooterCommand(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	sep := string(os.PathListSeparator)
	req := views.ExecRequestMsg{Namespace: "shop", Pod: "api-7d9f", Container: "app"}
	cases := []struct {
		name    string
		client  *k8s.Client
		flags   sessionFlags
		wantEnv []string // KUBECONFIG entries in the child's environment
	}{
		{
			name:    "current context",
			client:  &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: []string{"/home/me/.kube/config"}},
			wantEnv: []string{"KUBECONFIG=/home/me/.kube/config"},
		},
		{
			name:    "switched context with a space in its name",
			client:  &k8s.Client{Context: "it's prod", CurrentContext: "dev", KubeconfigPaths: []string{"/home/me/.kube/config"}},
			wantEnv: []string{"KUBECONFIG=/home/me/.kube/config"},
		},
		{
			name: "k3s fallback",
			client: &k8s.Client{
				Context: "default", CurrentContext: "default", KubeconfigPaths: []string{"/etc/rancher/k3s/k3s.yaml"},
				Source: k8s.Source{Paths: []string{"/etc/rancher/k3s/k3s.yaml"}, Origin: k8s.OriginK3s},
			},
			wantEnv: []string{"KUBECONFIG=/etc/rancher/k3s/k3s.yaml"},
		},
		{
			name:    "several files a bare kubectl would not load",
			client:  &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: []string{"/a/one", "/b/two"}},
			wantEnv: []string{"KUBECONFIG=/a/one" + sep + "/b/two"},
		},
		{
			name:    "context named on the command line",
			client:  &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: []string{"/home/me/.kube/config"}},
			flags:   sessionFlags{context: true},
			wantEnv: []string{"KUBECONFIG=/home/me/.kube/config"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := New(nil, nil)
			m.flags = tc.flags
			m = m.withClient(tc.client)
			m.kubectlSession = newKubectlSession(tc.client, tc.flags, func(string) string { return "" }, "/home/me")
			m = withLookPath(m, foundAt(fakeKubectlPath), []string{"PATH=/bin", "KUBECONFIG=/stale"})

			c, err := m.execCommand(req)
			if err != nil {
				t.Fatalf("execCommand: %v", err)
			}
			shown := m.decorate(mustExec(t, req))
			words := shellSplit(t, shown.String())
			env, argv := splitAssignments(words)
			if c.Args[0] != fakeKubectlPath || !slices.Equal(argv[1:], c.Args[1:]) || argv[0] != "kubectl" {
				t.Errorf("footer %q splits into %q, exec runs %q", shown.String(), argv, c.Args)
			}
			for _, kv := range env {
				if !slices.Contains(c.Env, kv) {
					t.Errorf("footer assigns %s, the child's env lacks it", kv)
				}
			}
			if got := kubeconfigEnv(c.Env); !slices.Equal(got, tc.wantEnv) {
				t.Errorf("KUBECONFIG = %v, want %v", got, tc.wantEnv)
			}
		})
	}
}

func mustExec(t *testing.T, req views.ExecRequestMsg) kubectl.Cmd {
	t.Helper()
	cmd, err := views.ExecCommand(req)
	if err != nil {
		t.Fatalf("ExecCommand: %v", err)
	}
	return cmd
}

// splitAssignments separates the leading NAME=value words of a shell line
// from the command.
func splitAssignments(words []string) (env, argv []string) {
	for i, w := range words {
		name, _, ok := strings.Cut(w, "=")
		if !ok || name == "" || strings.ContainsAny(name, "/- ") {
			return words[:i], words[i:]
		}
	}
	return words, nil
}

// shellSplit asks sh to word-split line and returns the words.
func shellSplit(t *testing.T, line string) []string {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("no sh on PATH")
	}
	script := `set -- ` + line + `; for w in "$@"; do printf '%s\000' "$w"; done`
	out, err := exec.Command("sh", "-c", script).Output() //nolint:gosec // test drives a fixed shell
	if err != nil {
		t.Fatalf("sh -c %q: %v", script, err)
	}
	parts := bytes.Split(out, []byte{0})
	words := make([]string, 0, len(parts))
	for _, p := range parts[:len(parts)-1] {
		words = append(words, string(p))
	}
	return words
}

func TestExecRequestErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		look func(string) (string, error)
		req  views.ExecRequestMsg
		want string
	}{
		{
			name: "neither kubectl nor k3s on PATH", look: notFound,
			req: views.ExecRequestMsg{Namespace: "ns", Pod: "pod-1"}, want: "exec: kubectl not found on PATH",
		},
		{
			name: "lookup failure", look: func(string) (string, error) { return "", errors.New("permission denied") },
			req: views.ExecRequestMsg{Namespace: "ns", Pod: "pod-1"}, want: "exec: find kubectl: permission denied",
		},
		{
			name: "pod name that reads as a flag", look: foundAt(fakeKubectlPath),
			req: views.ExecRequestMsg{Namespace: "ns", Pod: "--all"}, want: `exec: invalid name: pod "--all"`,
		},
		{
			name: "bad container", look: foundAt(fakeKubectlPath),
			req: views.ExecRequestMsg{Namespace: "ns", Pod: "pod-1", Container: "A B"}, want: `exec: invalid name: container "A B"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := withLookPath(New(nil, nil), tc.look, nil)
			upd, cmd := m.Update(tc.req)
			msg, ok := upd.(Model).status.Current(time.Now())
			if !ok || !strings.HasPrefix(msg.Text, tc.want) {
				t.Fatalf("status = %+v, %v; want it to start with %q", msg, ok, tc.want)
			}
			if cmd == nil {
				t.Fatal("expected the status expiry timer, got nil cmd")
			}
		})
	}
}

// TestExecRequestWithoutKubectlShowsFooterError drives the real exec.LookPath
// against an empty PATH, so it cannot run in parallel with other tests.
func TestExecRequestWithoutKubectlShowsFooterError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	m := New(nil, nil)
	upd, cmd := m.Update(views.ExecRequestMsg{Namespace: "ns", Pod: "pod-1"})
	m = upd.(Model)
	if msg, ok := m.status.Current(time.Now()); !ok || !strings.Contains(msg.Text, "kubectl not found on PATH") {
		t.Fatalf("status = %+v, %v; want it to mention kubectl not found on PATH", msg, ok)
	}
	if cmd == nil {
		t.Fatal("expected the status expiry timer, got nil cmd")
	}
}

// TestExecDoneReportsAFailedShell: the shell's exit error lands in the
// status row once the TUI is back; a clean exit says nothing.
func TestExecDoneReportsAFailedShell(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want string // "" means no status
	}{
		{name: "clean exit", err: nil},
		{name: "failed shell", err: errors.New("exit status 137"), want: "exec: exit status 137"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			upd, _ := New(nil, nil).Update(execDoneMsg{err: tc.err})
			msg, ok := upd.(Model).status.Current(time.Now())
			if tc.want == "" {
				if ok {
					t.Fatalf("status = %+v after a clean exit", msg)
				}
				return
			}
			if !ok || msg.Text != tc.want {
				t.Fatalf("status = %+v, %v; want %q", msg, ok, tc.want)
			}
		})
	}
}

// TestExecReadsTheShellEnvironment builds the model the way main does, with
// WithFlags and the production session wiring, so the $KUBECONFIG and home
// directory lookups behind the footer command are exercised. It sets the
// environment, so it cannot run in parallel.
func TestExecReadsTheShellEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	home := t.TempDir()
	defaultFile := filepath.Join(home, ".kube", "config")
	envFile := filepath.Join(t.TempDir(), "env.yaml")
	otherFile := filepath.Join(t.TempDir(), "other.yaml")
	req := views.ExecRequestMsg{Namespace: "shop", Pod: "api-7d9f"}

	cases := []struct {
		name       string
		kubeconfig string // $KUBECONFIG
		path       string // the file k4s loaded
		opts       cli.Options
		wantFlag   bool // the pasted command names --kubeconfig
	}{
		{name: "file from KUBECONFIG", kubeconfig: envFile, path: envFile},
		{name: "default file under HOME", path: defaultFile},
		{name: "file a bare kubectl would not load", kubeconfig: envFile, path: otherFile, wantFlag: true},
		{name: "file named on the command line", kubeconfig: envFile, path: envFile, opts: cli.Options{Kubeconfig: envFile}, wantFlag: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			t.Setenv(kubectl.EnvKubeconfig, tc.kubeconfig)
			c := &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: []string{tc.path}}
			m := New(c, nil, WithFlags(tc.opts))
			m = withLookPath(m, foundAt(fakeKubectlPath), []string{"PATH=/bin"})

			shown := m.decorate(mustExec(t, req)).String()
			if got := strings.Contains(shown, "--kubeconfig "+tc.path); got != tc.wantFlag {
				t.Errorf("%q names --kubeconfig = %v, want %v", shown, got, tc.wantFlag)
			}
			cmd, err := m.execCommand(req)
			if err != nil {
				t.Fatalf("execCommand: %v", err)
			}
			if got, want := kubeconfigEnv(cmd.Env), []string{"KUBECONFIG=" + tc.path}; !slices.Equal(got, want) {
				t.Errorf("KUBECONFIG = %v, want %v", got, want)
			}
		})
	}
}
