package kubectl_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

func TestCmdRendering(t *testing.T) {
	t.Parallel()

	list := strings.Join([]string{"/a/one", "/b/two"}, string(filepath.ListSeparator))
	tests := []struct {
		name     string
		cmd      kubectl.Cmd
		argv     []string
		env      []string
		str      string
		display  string
		zeroWant bool
	}{
		{
			name:     "zero value renders nothing",
			cmd:      kubectl.Cmd{},
			zeroWant: true,
		},
		{
			name:    "plain words stay bare",
			cmd:     kubectl.New("get", "pods", "-n", "k4s-demo"),
			argv:    []string{"kubectl", "get", "pods", "-n", "k4s-demo"},
			str:     "kubectl get pods -n k4s-demo",
			display: "kubectl get pods -n k4s-demo",
		},
		{
			name:    "approx marks only the display form",
			cmd:     kubectl.Cmd{Args: []string{"logs", "-f", "-l", "app=web"}, Approx: true},
			argv:    []string{"kubectl", "logs", "-f", "-l", "app=web"},
			str:     "kubectl logs -f -l app=web",
			display: "≈ kubectl logs -f -l app=web",
		},
		{
			name:    "selector with spaces is single-quoted",
			cmd:     kubectl.New("logs", "-l", "app in (a,b)"),
			argv:    []string{"kubectl", "logs", "-l", "app in (a,b)"},
			str:     "kubectl logs -l 'app in (a,b)'",
			display: "kubectl logs -l 'app in (a,b)'",
		},
		{
			name:    "context and single kubeconfig lead the flags",
			cmd:     kubectl.New("get", "pods").WithContext("prod").WithKubeconfig([]string{"/etc/rancher/k3s/k3s.yaml"}),
			argv:    []string{"kubectl", "--kubeconfig", "/etc/rancher/k3s/k3s.yaml", "--context", "prod", "get", "pods"},
			str:     "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml --context prod get pods",
			display: "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml --context prod get pods",
		},
		{
			name:    "several kubeconfig files become a KUBECONFIG assignment",
			cmd:     kubectl.New("get", "pods").WithKubeconfig([]string{"/a/one", "/b/two"}),
			argv:    []string{"kubectl", "get", "pods"},
			env:     []string{"KUBECONFIG=" + list},
			str:     "KUBECONFIG=" + list + " kubectl get pods",
			display: "KUBECONFIG=" + list + " kubectl get pods",
		},
		{
			name:    "kubeconfig path with a space is quoted",
			cmd:     kubectl.New("get", "ns").WithKubeconfig([]string{"/home/me/my configs/k3s.yaml"}),
			argv:    []string{"kubectl", "--kubeconfig", "/home/me/my configs/k3s.yaml", "get", "ns"},
			str:     "kubectl --kubeconfig '/home/me/my configs/k3s.yaml' get ns",
			display: "kubectl --kubeconfig '/home/me/my configs/k3s.yaml' get ns",
		},
		{
			name:    "context name with a quote survives",
			cmd:     kubectl.New("get", "ns").WithContext("it's-prod"),
			argv:    []string{"kubectl", "--context", "it's-prod", "get", "ns"},
			str:     `kubectl --context 'it'\''s-prod' get ns`,
			display: `kubectl --context 'it'\''s-prod' get ns`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.cmd.IsZero(); got != tt.zeroWant {
				t.Errorf("IsZero() = %v, want %v", got, tt.zeroWant)
			}
			if got := tt.cmd.Argv(); !slices.Equal(got, tt.argv) {
				t.Errorf("Argv() = %q, want %q", got, tt.argv)
			}
			if got := tt.cmd.Env(); !slices.Equal(got, tt.env) {
				t.Errorf("Env() = %q, want %q", got, tt.env)
			}
			if got := tt.cmd.String(); got != tt.str {
				t.Errorf("String() = %q, want %q", got, tt.str)
			}
			if got := tt.cmd.Display(); got != tt.display {
				t.Errorf("Display() = %q, want %q", got, tt.display)
			}
		})
	}
}

func TestCmdWithersDoNotShareState(t *testing.T) {
	t.Parallel()

	args := []string{"get", "pods"}
	base := kubectl.New(args...)
	args[0] = "delete"
	if got := base.String(); got != "kubectl get pods" {
		t.Fatalf("New kept the caller's slice: %q", got)
	}

	paths := []string{"/a/one"}
	withCtx := base.WithContext("prod")
	withCfg := withCtx.WithKubeconfig(paths)
	paths[0] = "/tampered"

	if got := base.String(); got != "kubectl get pods" {
		t.Errorf("base changed: %q", got)
	}
	if got := withCtx.String(); got != "kubectl --context prod get pods" {
		t.Errorf("WithContext result changed: %q", got)
	}
	if got := withCfg.String(); got != "kubectl --kubeconfig /a/one --context prod get pods" {
		t.Errorf("WithKubeconfig kept the caller's slice: %q", got)
	}

	argv := withCfg.Argv()
	argv[1] = "--tampered"
	if got := withCfg.Argv()[1]; got != "--kubeconfig" {
		t.Errorf("Argv() exposed internal state: %q", got)
	}
}

func TestCmdWithersClear(t *testing.T) {
	t.Parallel()

	cmd := kubectl.New("get", "pods").
		WithContext("prod").
		WithKubeconfig([]string{"/a/one", "/b/two"}).
		WithContext("").
		WithKubeconfig(nil)
	if got := cmd.String(); got != "kubectl get pods" {
		t.Errorf("String() = %q, want the bare command", got)
	}
	if env := cmd.Env(); env != nil {
		t.Errorf("Env() = %q, want nil", env)
	}
}

func TestCmdKubeconfigPathsAreCleaned(t *testing.T) {
	t.Parallel()

	cmd := kubectl.New("get", "pods").WithKubeconfig([]string{"/etc/rancher/./k3s//k3s.yaml", ""})
	want := "kubectl --kubeconfig /etc/rancher/k3s/k3s.yaml get pods"
	if got := cmd.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
