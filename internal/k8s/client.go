// Package k8s wraps client-go and exposes the narrow surface k4s needs.
// Future additions (dynamic client, discovery, watcher caches) hang off Client.
package k8s

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Client wraps a Kubernetes clientset together with the kubeconfig context it
// was built from. Clientset is typed as the kubernetes.Interface so tests can
// substitute fake.NewSimpleClientset.
//
// RestConfig is retained so callers that need a raw *rest.Config — currently
// the port-forward path, which builds an SPDY dialer — do not have to
// re-parse the kubeconfig. It is nil only when the Client was constructed
// without a kubeconfig (e.g. fake clients in tests).
//
// KubeconfigPaths lists the files the config was loaded from, in precedence
// order, so a kubectl subprocess can be pointed at exactly the same config.
type Client struct {
	Clientset       kubernetes.Interface
	Context         string
	RestConfig      *rest.Config
	KubeconfigPaths []string
}

// LoadFromKubeconfig builds a Client from a kubeconfig file.
//
// Resolution order matches kubectl semantics:
//   - explicitPath, if non-empty
//   - $KUBECONFIG (may be a colon-separated list)
//   - $HOME/.kube/config
func LoadFromKubeconfig(explicitPath string) (*Client, error) {
	return LoadFromKubeconfigContext(explicitPath, "")
}
