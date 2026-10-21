// Package k8s wraps client-go and exposes the narrow surface k4s needs.
// Future additions (dynamic client, discovery, watcher caches) hang off Client.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Origin says how k4s found its kubeconfig. It decides whether a kubectl
// command pasted into a shell needs --kubeconfig to reach the same cluster.
type Origin int

const (
	// OriginKubectl is what bare kubectl loads: $KUBECONFIG, else
	// ~/.kube/config.
	OriginKubectl Origin = iota
	// OriginFlag is a file named with --kubeconfig.
	OriginFlag
	// OriginK3s is the k3s server's own kubeconfig
	// (/etc/rancher/k3s/k3s.yaml), which upstream kubectl never reads.
	OriginK3s
)

// Source names the kubeconfig files a Client loads. The zero value is
// client-go's default lookup, the same one bare kubectl does.
type Source struct {
	// Paths in precedence order. Under OriginKubectl missing files are
	// skipped, as kubectl does; otherwise Paths holds the one file, and it
	// must exist.
	Paths  []string
	Origin Origin
}

// KubeconfigArg is the --kubeconfig value a kubectl command needs to load
// the same file, or "" when bare kubectl already loads it.
func (s Source) KubeconfigArg() string {
	if s.Origin == OriginKubectl || len(s.Paths) == 0 {
		return ""
	}
	return s.Paths[0]
}

// Client wraps a Kubernetes clientset together with the kubeconfig context it
// was built from. Clientset is typed as the kubernetes.Interface so tests can
// substitute fake.NewSimpleClientset.
//
// RestConfig is retained so callers that need a raw *rest.Config — currently
// the port-forward path, which builds an SPDY dialer — do not have to
// re-parse the kubeconfig. It is nil only when the Client was constructed
// without a kubeconfig (e.g. fake clients in tests). Its exec credential
// plugin never gets the terminal's stdin: see Interactive.
//
// KubeconfigPaths lists the files the config was loaded from, in precedence
// order, so a kubectl subprocess can be pointed at exactly the same config.
type Client struct {
	Clientset       kubernetes.Interface
	Context         string
	RestConfig      *rest.Config
	KubeconfigPaths []string

	// Source is where the kubeconfig came from; SwitchContext reloads it.
	Source Source
	// ContextNamespace is the namespace the kubeconfig context sets, "" if
	// it sets none.
	ContextNamespace string
	// Namespace is where k4s starts: ContextNamespace unless the command
	// line overrode it (see WithNamespace). "" means all namespaces.
	Namespace string
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

// Load builds a Client for contextName, or for the current-context when
// contextName is "", from the kubeconfig src names. The Client starts in
// the context's namespace.
func Load(src Source, contextName string) (*Client, error) {
	rules := src.rules()
	raw, err := rules.Load()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	target := contextName
	if target == "" {
		target = raw.CurrentContext
	}
	if target == "" {
		return nil, fmt.Errorf("kubeconfig has no current-context")
	}
	kctx, ok := raw.Contexts[target]
	if !ok {
		return nil, fmt.Errorf("context %q not found in kubeconfig", target)
	}
	if kctx.Namespace != "" {
		if err := ValidateNamespace(kctx.Namespace); err != nil {
			return nil, fmt.Errorf("context %q: %w", target, err)
		}
	}
	cfg, err := buildConfig(*raw, target, rules)
	if err != nil {
		return nil, err
	}
	cfg = withStdinUnavailable(cfg, true, loginHint(target, src))
	cs, err := newClientset(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{
		Clientset:        cs,
		Context:          target,
		RestConfig:       cfg,
		KubeconfigPaths:  rules.GetLoadingPrecedence(),
		Source:           src,
		ContextNamespace: kctx.Namespace,
		Namespace:        kctx.Namespace,
	}, nil
}

// SwitchContext loads context name from the same kubeconfig as c. The new
// Client starts in that context's namespace.
func (c *Client) SwitchContext(name string) (*Client, error) {
	return Load(c.Source, name)
}

// WithNamespace returns a copy of c that starts in ns ("" for all
// namespaces).
func (c *Client) WithNamespace(ns string) *Client {
	out := *c
	out.Namespace = ns
	return &out
}

// KubeconfigArg is the --kubeconfig value a kubectl command needs to reach
// the same cluster as c, or "" when bare kubectl already does.
func (c *Client) KubeconfigArg() string {
	if c == nil {
		return ""
	}
	return c.Source.KubeconfigArg()
}

// Interactive returns a copy of c whose exec credential plugin may prompt on
// the terminal (SSO device codes, MFA). Only the preflight that runs before
// the TUI takes the terminal may use it: a plugin reading stdin under the
// TUI would steal its keystrokes. Without an exec plugin it returns c.
//
// client-go caches one authenticator per plugin config, so the copy and c
// each run the plugin once; plugins that keep no token cache of their own
// therefore prompt here and then fail under c.
func (c *Client) Interactive() (*Client, error) {
	if c == nil || c.RestConfig == nil || c.RestConfig.ExecProvider == nil {
		return c, nil
	}
	cfg := withStdinUnavailable(c.RestConfig, false, "")
	cs, err := newClientset(cfg)
	if err != nil {
		return nil, err
	}
	out := *c
	out.Clientset = cs
	out.RestConfig = cfg
	return &out, nil
}

// ServerVersion asks the API server for /version. It is the cheapest
// authenticated request, so it also runs a credential plugin once.
func (c *Client) ServerVersion(ctx context.Context) (*apiversion.Info, error) {
	rc := c.Clientset.Discovery().RESTClient()
	if rc == nil {
		// A fake clientset has no REST client; it answers from memory.
		info, err := c.Clientset.Discovery().ServerVersion()
		if err != nil {
			return nil, fmt.Errorf("get server version: %w", err)
		}
		return info, nil
	}
	body, err := rc.Get().AbsPath("/version").Do(ctx).Raw()
	if err != nil {
		return nil, fmt.Errorf("get server version: %w", err)
	}
	var info apiversion.Info
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("decode server version: %w", err)
	}
	return &info, nil
}
