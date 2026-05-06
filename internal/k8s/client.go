// Package k8s wraps client-go and exposes the narrow surface k4s needs.
// Future additions (dynamic client, discovery, watcher caches) hang off Client.
package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps a Kubernetes clientset together with the kubeconfig context it
// was built from. Clientset is typed as the kubernetes.Interface so tests can
// substitute fake.NewSimpleClientset.
type Client struct {
	Clientset kubernetes.Interface
	Context   string
}

// LoadFromKubeconfig builds a Client from a kubeconfig file.
//
// Resolution order matches kubectl semantics:
//   - explicitPath, if non-empty
//   - $KUBECONFIG (may be a colon-separated list)
//   - $HOME/.kube/config
func LoadFromKubeconfig(explicitPath string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}

	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	if raw.CurrentContext == "" {
		return nil, fmt.Errorf("kubeconfig has no current-context")
	}

	cfg, err := clientcmd.NewNonInteractiveClientConfig(*raw, raw.CurrentContext, nil, rules).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config: %w", err)
	}

	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new clientset: %w", err)
	}

	return &Client{Clientset: cs, Context: raw.CurrentContext}, nil
}
