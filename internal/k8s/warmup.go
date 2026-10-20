package k8s

import (
	"context"
	"fmt"
)

// NeedsLogin reports whether the client authenticates through an exec or
// auth-provider credential plugin (EKS, GKE, OIDC), which may prompt the
// user the first time it runs.
func (c *Client) NeedsLogin() bool {
	if c == nil || c.RestConfig == nil {
		return false
	}
	return c.RestConfig.ExecProvider != nil || c.RestConfig.AuthProvider != nil
}

// WarmUp sends one cheap request so credential plugins run now, while the
// terminal still belongs to the user and a login prompt can be seen and
// answered, instead of later behind the TUI.
func (c *Client) WarmUp(ctx context.Context) error {
	rc := c.Clientset.Discovery().RESTClient()
	if rc == nil {
		return nil
	}
	if err := rc.Get().AbsPath("/version").Do(ctx).Error(); err != nil {
		return fmt.Errorf("warm up credentials: %w", err)
	}
	return nil
}
