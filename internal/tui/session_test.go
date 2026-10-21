package tui

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// flagValue returns the value after flag in argv, "" when it is absent.
func flagValue(argv []string, flag string) string {
	i := slices.Index(argv, flag)
	if i < 0 || i+1 >= len(argv) {
		return ""
	}
	return argv[i+1]
}

// TestKubeconfigRuleAgreesWithSource pins the one rule for --kubeconfig: a
// footer command carries it exactly when k8s.Source.KubeconfigArg says a
// bare kubectl would load another file (an explicit --kubeconfig, the k3s
// fallback), and never for the files a bare kubectl reads anyway.
func TestKubeconfigRuleAgreesWithSource(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	homeConfig := filepath.Join(home, ".kube", "config")
	if err := os.MkdirAll(filepath.Dir(homeConfig), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := tuitest.WriteKubeconfig(filepath.Dir(homeConfig), "alpha", "alpha"); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	listA, err := tuitest.WriteKubeconfig(other, "alpha", "alpha")
	if err != nil {
		t.Fatal(err)
	}
	listB := filepath.Join(other, "absent")
	k3s, err := tuitest.WriteKubeconfig(t.TempDir(), "default", "default")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name           string
		src            k8s.Source
		kubeconfigEnv  string
		kubeconfigFlag bool
	}{
		{name: "home kubeconfig", src: k8s.Source{Paths: []string{homeConfig}}},
		{name: "KUBECONFIG list", src: k8s.Source{Paths: []string{listA, listB}}, kubeconfigEnv: listA + string(os.PathListSeparator) + listB},
		{name: "KUBECONFIG with a repeat", src: k8s.Source{Paths: []string{listA}}, kubeconfigEnv: listA + string(os.PathListSeparator) + listA},
		{name: "explicit --kubeconfig", src: k8s.Source{Paths: []string{listA}, Origin: k8s.OriginFlag}, kubeconfigFlag: true},
		{name: "explicit --kubeconfig naming the home file", src: k8s.Source{Paths: []string{homeConfig}, Origin: k8s.OriginFlag}, kubeconfigFlag: true},
		{name: "explicit file without the flag recorded", src: k8s.Source{Paths: []string{homeConfig}, Origin: k8s.OriginFlag}},
		{name: "k3s fallback", src: k8s.Source{Paths: []string{k3s}, Origin: k8s.OriginK3s}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := k8s.Load(tc.src, "")
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			getenv := func(key string) string {
				if key == kubectl.EnvKubeconfig {
					return tc.kubeconfigEnv
				}
				return ""
			}
			sess := newKubectlSession(c, sessionFlags{kubeconfig: tc.kubeconfigFlag}, getenv, home)
			got := kubectl.Decorate(kubectl.New("get", "pods"), sess)

			want := ""
			if arg := tc.src.KubeconfigArg(); arg != "" {
				abs, err := filepath.Abs(arg)
				if err != nil {
					t.Fatal(err)
				}
				want = abs
			}
			if pinned := flagValue(got.Argv(), "--kubeconfig"); pinned != want {
				t.Errorf("--kubeconfig = %q, want %q (KubeconfigArg %q); command %q", pinned, want, tc.src.KubeconfigArg(), got.String())
			}
			if env := got.Env(); env != nil {
				t.Errorf("Env = %v, want none", env)
			}
		})
	}
}

func TestKubectlSessionWithoutAClient(t *testing.T) {
	t.Parallel()

	sess := newKubectlSession(nil, sessionFlags{context: true, kubeconfig: true}, func(string) string { return "" }, "/home/me")
	if got := kubectl.Decorate(kubectl.New("get", "pods"), sess).String(); got != "kubectl get pods" {
		t.Fatalf("without a client the command is decorated: %q", got)
	}
}

func TestKubectlSessionNamesTheContext(t *testing.T) {
	t.Parallel()

	c := &k8s.Client{Context: "prod", CurrentContext: "dev", KubeconfigPaths: []string{"/home/me/.kube/config"}}
	getenv := func(string) string { return "" }
	cases := []struct {
		name  string
		c     *k8s.Client
		flags sessionFlags
		want  string
	}{
		{name: "switched context", c: c, want: "kubectl --context prod get pods"},
		{name: "current context", c: &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: c.KubeconfigPaths}, want: "kubectl get pods"},
		{
			name: "current context named on the command line", flags: sessionFlags{context: true},
			c: &k8s.Client{Context: "dev", CurrentContext: "dev", KubeconfigPaths: c.KubeconfigPaths}, want: "kubectl --context dev get pods",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sess := newKubectlSession(tc.c, tc.flags, getenv, "/home/me")
			if got := kubectl.Decorate(kubectl.New("get", "pods"), sess).String(); got != tc.want {
				t.Fatalf("command = %q, want %q", got, tc.want)
			}
		})
	}
}
