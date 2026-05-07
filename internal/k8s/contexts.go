package k8s

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Context is a presentation-friendly view of a kubeconfig context — just
// the bits the TUI needs to render a picker (name + cluster + user) and the
// "is this one currently selected?" flag.
type Context struct {
	Name      string
	Cluster   string
	AuthInfo  string
	Namespace string
	Current   bool
}

// ListContexts reads the kubeconfig and returns every defined context.
// Resolution order matches LoadFromKubeconfig (explicit path → $KUBECONFIG
// → ~/.kube/config). The slice preserves whatever order the file uses;
// callers wanting alphabetical output should sort.
func ListContexts(explicitPath string) ([]Context, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	out := make([]Context, 0, len(raw.Contexts))
	for name, c := range raw.Contexts {
		out = append(out, Context{
			Name:      name,
			Cluster:   c.Cluster,
			AuthInfo:  c.AuthInfo,
			Namespace: c.Namespace,
			Current:   name == raw.CurrentContext,
		})
	}
	return out, nil
}

// LoadFromKubeconfigContext is the explicit-context cousin of
// LoadFromKubeconfig: same lookup rules, but the named context wins over
// whatever the file's current-context says. Empty contextName falls back
// to the current-context to keep behaviour identical to LoadFromKubeconfig.
func LoadFromKubeconfigContext(explicitPath, contextName string) (*Client, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if explicitPath != "" {
		rules.ExplicitPath = explicitPath
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	target := contextName
	if target == "" {
		target = raw.CurrentContext
	}
	if target == "" {
		return nil, fmt.Errorf("kubeconfig has no current-context and no explicit context requested")
	}
	if _, ok := raw.Contexts[target]; !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", target)
	}
	cfg, err := clientcmd.NewNonInteractiveClientConfig(*raw, target, nil, rules).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("build client config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("new clientset: %w", err)
	}
	return &Client{Clientset: cs, Context: target}, nil
}
