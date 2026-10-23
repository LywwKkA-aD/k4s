package kubectl_test

import (
	"errors"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

func TestCheckFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cmd  kubectl.Cmd
		flag string // "" when the command passes
	}{
		{"zero command", kubectl.Cmd{}, ""},
		{"plain list", kubectl.New("get", "pods", "-A"), ""},
		{"namespace and container", kubectl.New("logs", "web", "-n", "shop", "-c", "app", "--previous"), ""},
		{"long flags k4s sets", kubectl.New("top", "pods", "--sort-by=cpu", "--field-selector", "metadata.name=web"), ""},
		{"output template", kubectl.New("get", "node", "n1", "-o", `jsonpath={.metadata.annotations.etcd\.k3s\.cattle\.io/x}`), ""},
		{"exec shorthand cluster", kubectl.New("exec", "-it", "-n", "shop", "web", "--", "sh"), ""},
		{"after double dash", kubectl.New("exec", "-n", "shop", "web", "--", "sh", "-c", "--server=x -s y"), ""},
		{"server inline", kubectl.New("describe", "pod", "--server=https://evil", "-n", "ns"), "--server"},
		{"server split", kubectl.New("describe", "pod", "--server", "https://evil"), "--server"},
		{"kubeconfig", kubectl.New("describe", "pod", "--kubeconfig=/tmp/evil"), "--kubeconfig"},
		{"context", kubectl.New("get", "pods", "--context", "prod"), "--context"},
		{"cluster", kubectl.New("get", "pods", "--cluster=x"), "--cluster"},
		{"user", kubectl.New("get", "pods", "--user=x"), "--user"},
		{"token", kubectl.New("get", "pods", "--token=x"), "--token"},
		{"as", kubectl.New("get", "pods", "--as=system:admin"), "--as"},
		{"as-group", kubectl.New("get", "pods", "--as-group=system:masters"), "--as-group"},
		{"as-uid", kubectl.New("get", "pods", "--as-uid=0"), "--as-uid"},
		{"username", kubectl.New("get", "pods", "--username=x"), "--username"},
		{"password", kubectl.New("get", "pods", "--password=x"), "--password"},
		{"certificate-authority", kubectl.New("get", "pods", "--certificate-authority=/x"), "--certificate-authority"},
		{"client-certificate", kubectl.New("get", "pods", "--client-certificate=/x"), "--client-certificate"},
		{"client-key", kubectl.New("get", "pods", "--client-key=/x"), "--client-key"},
		{"insecure-skip-tls-verify", kubectl.New("get", "pods", "--insecure-skip-tls-verify"), "--insecure-skip-tls-verify"},
		{"tls-server-name", kubectl.New("get", "pods", "--tls-server-name=x"), "--tls-server-name"},
		{"log-file", kubectl.New("get", "pods", "--log-file=/x"), "--log-file"},
		{"log-dir", kubectl.New("get", "pods", "--log-dir=/x"), "--log-dir"},
		{"profile-output", kubectl.New("get", "pods", "--profile-output=/x"), "--profile-output"},
		{"cache-dir", kubectl.New("get", "pods", "--cache-dir=/x"), "--cache-dir"},
		{"kuberc", kubectl.New("get", "pods", "--kuberc=/x"), "--kuberc"},
		{"short server split", kubectl.New("get", "pods", "-s", "https://evil"), "-s"},
		{"short server with =", kubectl.New("get", "pods", "-s=https://evil"), "-s"},
		{"short server glued", kubectl.New("get", "pods", "-shttps://evil"), "-s"},
		{"short server in a cluster", kubectl.New("get", "pods", "-As=https://evil"), "-s"},
		{"before a later double dash", kubectl.New("exec", "--token=x", "web", "--", "sh"), "--token"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := kubectl.CheckFlags(tt.cmd)
			if tt.flag == "" {
				if err != nil {
					t.Fatalf("CheckFlags(%q) = %v, want nil", tt.cmd.Args, err)
				}
				return
			}
			var fe *kubectl.FlagError
			if !errors.As(err, &fe) || fe.Flag != tt.flag {
				t.Fatalf("CheckFlags(%q) = %v, want a FlagError for %s", tt.cmd.Args, err, tt.flag)
			}
			if !errors.Is(err, kubectl.ErrSessionFlag) {
				t.Errorf("error %v does not wrap ErrSessionFlag", err)
			}
			if want := "the command sets " + tt.flag; err.Error() != want {
				t.Errorf("Error() = %q, want %q", err.Error(), want)
			}
		})
	}
}

// Decorate keeps --kubeconfig and --context outside Args, so a decorated
// command still passes.
func TestCheckFlagsAcceptsDecoratedCommands(t *testing.T) {
	t.Parallel()

	s := kubectl.Session{
		Context: "prod", CurrentContext: "dev", ContextFlag: true,
		Kubeconfig: []string{"/home/u/.kube/a"}, BareKubeconfig: []string{"/home/u/.kube/config"}, KubeconfigFlag: true,
	}
	c := kubectl.Decorate(kubectl.New("get", "pods", "-n", "shop"), s)
	if c.String() != "kubectl --kubeconfig /home/u/.kube/a --context prod get pods -n shop" {
		t.Fatalf("decorated = %q", c.String())
	}
	if err := kubectl.CheckFlags(c); err != nil {
		t.Errorf("CheckFlags(decorated) = %v, want nil", err)
	}
}
