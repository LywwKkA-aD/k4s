package k8s

import (
	"fmt"
	"slices"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
	"github.com/LywwKkA-aD/k4s/internal/version"
)

// client-go's defaults (QPS 5, Burst 10) throttle a TUI that refreshes
// several lists, streams logs and polls metrics at the same time.
const (
	clientQPS   = 50
	clientBurst = 100
)

// rules turns s into client-go loading rules. ExplicitPath makes a missing
// file an error, which is what a named file needs; a precedence list skips
// missing files the way kubectl does with $KUBECONFIG.
func (s Source) rules() *clientcmd.ClientConfigLoadingRules {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	switch {
	case len(s.Paths) == 0:
	case s.Origin == OriginKubectl:
		rules.Precedence = slices.Clone(s.Paths)
	default:
		rules.ExplicitPath = s.Paths[0]
	}
	return rules
}

// buildConfig resolves contextName from an already-loaded kubeconfig into a
// rest.Config carrying k4s's client defaults.
func buildConfig(raw clientcmdapi.Config, contextName string, rules clientcmd.ClientConfigLoader) (*rest.Config, error) {
	cfg, err := clientcmd.NewNonInteractiveClientConfig(raw, contextName, nil, rules).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config: %w", err)
	}
	return withClientDefaults(cfg), nil
}

func withClientDefaults(cfg *rest.Config) *rest.Config {
	out := rest.CopyConfig(cfg)
	out.QPS = clientQPS
	out.Burst = clientBurst
	out.UserAgent = "k4s/" + version.String()
	return out
}

// withStdinUnavailable returns a copy of cfg whose exec credential plugin
// is told that stdin is (or is not) unavailable; msg is what client-go
// reports when a plugin insists on a terminal anyway. rest.CopyConfig
// shares the ExecProvider pointer, so it is copied too.
func withStdinUnavailable(cfg *rest.Config, unavailable bool, msg string) *rest.Config {
	out := rest.CopyConfig(cfg)
	if out.ExecProvider == nil {
		return out
	}
	ep := *out.ExecProvider
	ep.StdinUnavailable = unavailable
	ep.StdinUnavailableMessage = msg
	out.ExecProvider = &ep
	return out
}

// loginHint tells the user how to refresh credentials that need a terminal
// while the TUI owns it; r then retries the list.
func loginHint(contextName string, src Source) string {
	cmd := "kubectl --context " + kubectl.Quote(contextName)
	if arg := src.KubeconfigArg(); arg != "" {
		cmd += " --kubeconfig " + kubectl.Quote(arg)
	}
	return "log in first: run `" + cmd + " get ns` in a terminal, then press r"
}

// newClientset builds the clientset with exec credential plugin stderr
// captured: client-go's exec authenticator keeps the os.Stderr it sees here
// and hands it to every later plugin run, which happens while the TUI owns
// the terminal.
func newClientset(cfg *rest.Config) (*kubernetes.Clientset, error) {
	var cs *kubernetes.Clientset
	err := logsink.CapturePluginStderr(func() error {
		var err error
		cs, err = kubernetes.NewForConfig(cfg)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("new clientset: %w", err)
	}
	return cs, nil
}
