package tui

import (
	"errors"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func foundAt(path string) func(string) (string, error) {
	return func(string) (string, error) { return path, nil }
}

func kubeconfigEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		if strings.HasPrefix(kv, "KUBECONFIG=") {
			out = append(out, kv)
		}
	}
	return out
}

func TestNewExecCommand(t *testing.T) {
	t.Parallel()

	sep := string(os.PathListSeparator)
	req := views.ExecRequestMsg{Namespace: "ns", Pod: "pod-1", Container: "app"}
	wantArgs := []string{"kubectl", "exec", "-it", "--context", "staging", "-n", "ns", "-c", "app", "pod-1", "--", "sh"}

	cases := []struct {
		name    string
		client  *k8s.Client
		wantEnv []string // nil means the child inherits the parent environment
	}{
		{
			name:    "single kubeconfig",
			client:  &k8s.Client{Context: "staging", KubeconfigPaths: []string{"/home/u/.kube/config"}},
			wantEnv: []string{"KUBECONFIG=/home/u/.kube/config"},
		},
		{
			name:    "merged kubeconfig list keeps precedence order",
			client:  &k8s.Client{Context: "staging", KubeconfigPaths: []string{"/a/one", "/b/two"}},
			wantEnv: []string{"KUBECONFIG=/a/one" + sep + "/b/two"},
		},
		{
			name:   "no recorded paths inherits the environment",
			client: &k8s.Client{Context: "staging"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd, err := newExecCommand(foundAt("/usr/local/bin/kubectl"), tc.client, req)
			if err != nil {
				t.Fatalf("newExecCommand: %v", err)
			}
			if !slices.Equal(cmd.Args, wantArgs) {
				t.Errorf("Args = %v, want %v", cmd.Args, wantArgs)
			}
			if tc.wantEnv == nil {
				if cmd.Env != nil {
					t.Errorf("Env = %v, want nil (inherit)", cmd.Env)
				}
				return
			}
			if got := kubeconfigEnv(cmd.Env); !slices.Equal(got, tc.wantEnv) {
				t.Errorf("KUBECONFIG entries = %v, want %v", got, tc.wantEnv)
			}
			if len(cmd.Env) < len(os.Environ())-1 {
				t.Errorf("Env dropped the parent environment: %d entries, parent has %d", len(cmd.Env), len(os.Environ()))
			}
		})
	}
}

func TestExecEnvReplacesInheritedKubeconfig(t *testing.T) {
	t.Parallel()

	base := []string{"PATH=/bin", "KUBECONFIG=/etc/rancher/k3s/k3s.yaml", "HOME=/home/u"}
	got := execEnv(base, []string{"/home/u/.kube/config"})
	want := []string{"PATH=/bin", "HOME=/home/u", "KUBECONFIG=/home/u/.kube/config"}
	if !slices.Equal(got, want) {
		t.Fatalf("execEnv = %v, want %v", got, want)
	}
	if base[1] != "KUBECONFIG=/etc/rancher/k3s/k3s.yaml" {
		t.Fatalf("execEnv mutated its input: %v", base)
	}
}

func TestNewExecCommandKubectlMissing(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		lookErr error
		want    string
	}{
		{name: "not on PATH", lookErr: &exec.Error{Name: "kubectl", Err: exec.ErrNotFound}, want: "kubectl not found on PATH"},
		{name: "other lookup failure", lookErr: errors.New("permission denied"), want: "find kubectl: permission denied"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lookPath := func(string) (string, error) { return "", tc.lookErr }
			cmd, err := newExecCommand(lookPath, &k8s.Client{}, views.ExecRequestMsg{Namespace: "ns", Pod: "p"})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if cmd != nil {
				t.Fatalf("cmd = %v, want nil when kubectl cannot be found", cmd)
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

func TestWithoutEnvKey(t *testing.T) {
	t.Parallel()

	base := []string{"PATH=/bin", "KUBECONFIG=/a", "Kubeconfig=/b", "KUBECONFIGX=/c"}
	cases := []struct {
		name     string
		foldCase bool
		want     []string
	}{
		{name: "case-sensitive like unix", want: []string{"PATH=/bin", "Kubeconfig=/b", "KUBECONFIGX=/c"}},
		{name: "case-insensitive like windows", foldCase: true, want: []string{"PATH=/bin", "KUBECONFIGX=/c"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := withoutEnvKey(base, kubeconfigEV, tc.foldCase); !slices.Equal(got, tc.want) {
				t.Fatalf("withoutEnvKey = %v, want %v", got, tc.want)
			}
		})
	}
}
