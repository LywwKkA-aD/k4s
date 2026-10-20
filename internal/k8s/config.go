package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/logsink"
	"github.com/LywwKkA-aD/k4s/internal/version"
)

// client-go's defaults (QPS 5, Burst 10) throttle a TUI that refreshes
// several lists, streams logs and polls metrics at the same time.
const (
	clientQPS   = 50
	clientBurst = 100
)

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
