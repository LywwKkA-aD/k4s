package kubectl_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

func TestDecorate(t *testing.T) {
	t.Parallel()

	const (
		home = "/home/me/.kube/config"
		k3s  = "/etc/rancher/k3s/k3s.yaml"
	)
	join := func(p ...string) string { return strings.Join(p, string(filepath.ListSeparator)) }
	pods := kubectl.New("get", "pods", "-n", "k4s-demo")

	tests := []struct {
		name string
		cmd  kubectl.Cmd
		s    kubectl.Session
		want string
	}{
		{
			name: "same context and same files add nothing",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{home}, BareKubeconfig: []string{home}},
			want: "kubectl get pods -n k4s-demo",
		},
		{
			name: "context switched with :ctx",
			cmd:  pods,
			s:    kubectl.Session{Context: "prod", CurrentContext: "default", Kubeconfig: []string{home}, BareKubeconfig: []string{home}},
			want: "kubectl --context prod get pods -n k4s-demo",
		},
		{
			name: "--context passed even when it names the current context",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", ContextFlag: true, Kubeconfig: []string{home}, BareKubeconfig: []string{home}},
			want: "kubectl --context default get pods -n k4s-demo",
		},
		{
			name: "no active context never adds the flag",
			cmd:  pods,
			s:    kubectl.Session{CurrentContext: "default", ContextFlag: true},
			want: "kubectl get pods -n k4s-demo",
		},
		{
			name: "config subcommands never get --context",
			cmd:  kubectl.New("config", "use-context", "prod"),
			s:    kubectl.Session{Context: "prod", CurrentContext: "default", Kubeconfig: []string{k3s}, BareKubeconfig: []string{home}},
			want: "kubectl --kubeconfig " + k3s + " config use-context prod",
		},
		{
			name: "--kubeconfig passed for the file bare kubectl reads anyway",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{home}, BareKubeconfig: []string{home}, KubeconfigFlag: true},
			want: "kubectl --kubeconfig " + home + " get pods -n k4s-demo",
		},
		{
			name: "--kubeconfig passed for another file",
			cmd:  pods,
			s:    kubectl.Session{Context: "lab", CurrentContext: "lab", Kubeconfig: []string{"/tmp/lab.yaml"}, BareKubeconfig: []string{home}, KubeconfigFlag: true},
			want: "kubectl --kubeconfig /tmp/lab.yaml get pods -n k4s-demo",
		},
		{
			name: "k3s.yaml fallback",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{k3s}, BareKubeconfig: []string{home}},
			want: "kubectl --kubeconfig " + k3s + " get pods -n k4s-demo",
		},
		{
			name: "KUBECONFIG list narrowed to one file",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{"/a"}, BareKubeconfig: []string{"/a", "/b"}},
			want: "kubectl --kubeconfig /a get pods -n k4s-demo",
		},
		{
			name: "KUBECONFIG list narrowed to two files",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{"/a", "/b"}, BareKubeconfig: []string{"/a", "/b", "/c"}},
			want: "KUBECONFIG=" + join("/a", "/b") + " kubectl get pods -n k4s-demo",
		},
		{
			name: "same list in another order changes merge precedence",
			cmd:  pods,
			s:    kubectl.Session{Kubeconfig: []string{"/b", "/a"}, BareKubeconfig: []string{"/a", "/b"}},
			want: "KUBECONFIG=" + join("/b", "/a") + " kubectl get pods -n k4s-demo",
		},
		{
			name: "unclean spellings of the same file add nothing",
			cmd:  pods,
			s:    kubectl.Session{Kubeconfig: []string{"/home/me/.kube/./config"}, BareKubeconfig: []string{home}},
			want: "kubectl get pods -n k4s-demo",
		},
		{
			name: "no recorded kubeconfig adds nothing",
			cmd:  pods,
			s:    kubectl.Session{Context: "default", CurrentContext: "default", KubeconfigFlag: true, BareKubeconfig: []string{home}},
			want: "kubectl get pods -n k4s-demo",
		},
		{
			name: "both flags",
			cmd:  pods,
			s:    kubectl.Session{Context: "prod", CurrentContext: "default", Kubeconfig: []string{k3s}, BareKubeconfig: []string{home}},
			want: "kubectl --kubeconfig " + k3s + " --context prod get pods -n k4s-demo",
		},
		{
			name: "stale decoration is replaced",
			cmd:  pods.WithContext("old").WithKubeconfig([]string{"/old"}),
			s:    kubectl.Session{Context: "default", CurrentContext: "default", Kubeconfig: []string{home}, BareKubeconfig: []string{home}},
			want: "kubectl get pods -n k4s-demo",
		},
		{
			name: "zero command stays zero",
			cmd:  kubectl.Cmd{},
			s:    kubectl.Session{Context: "prod", CurrentContext: "default", Kubeconfig: []string{k3s}},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := kubectl.Decorate(tt.cmd, tt.s)
			if got.String() != tt.want {
				t.Errorf("Decorate() = %q, want %q", got.String(), tt.want)
			}
			if again := kubectl.Decorate(got, tt.s); again.String() != got.String() {
				t.Errorf("Decorate is not idempotent: %q then %q", got.String(), again.String())
			}
		})
	}
}

func TestDecorateKeepsFlagsAndArgs(t *testing.T) {
	t.Parallel()

	in := kubectl.Cmd{Args: []string{"delete", "pod", "web", "-n", "k4s-demo"}, Mutating: true, Approx: true}
	out := kubectl.Decorate(in, kubectl.Session{Context: "prod", CurrentContext: "default"})
	if !out.Mutating || !out.Approx {
		t.Errorf("Decorate dropped Mutating/Approx: %+v", out)
	}
	if !slices.Equal(out.Args, in.Args) {
		t.Errorf("Decorate changed Args: %q", out.Args)
	}
	if got := in.String(); got != "kubectl delete pod web -n k4s-demo" {
		t.Errorf("Decorate changed its input: %q", got)
	}
}

func TestBareKubeconfig(t *testing.T) {
	t.Parallel()

	join := func(p ...string) string { return strings.Join(p, string(filepath.ListSeparator)) }
	tests := []struct {
		name, env, home string
		want            []string
	}{
		{name: "unset falls back to home", home: "/home/me", want: []string{filepath.Join("/home/me", ".kube", "config")}},
		{name: "unset and no home", want: nil},
		{name: "single file", env: "/a", home: "/home/me", want: []string{"/a"}},
		{name: "list keeps order", env: join("/b", "/a"), home: "/home/me", want: []string{"/b", "/a"}},
		{name: "empty entries and duplicates drop", env: join("/a", "", "/b", "/a"), home: "/home/me", want: []string{"/a", "/b"}},
		// client-go uses a set KUBECONFIG as-is, even when it names no file.
		{name: "only separators loads nothing", env: join("", ""), home: "/home/me", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kubectl.BareKubeconfig(tt.env, tt.home); !slices.Equal(got, tt.want) {
				t.Errorf("BareKubeconfig(%q, %q) = %q, want %q", tt.env, tt.home, got, tt.want)
			}
		})
	}
}
