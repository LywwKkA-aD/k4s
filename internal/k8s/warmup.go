package k8s

// NeedsLogin reports whether the client authenticates through an exec or
// auth-provider credential plugin (EKS, GKE, OIDC), which may prompt the
// user the first time it runs.
func (c *Client) NeedsLogin() bool {
	if c == nil || c.RestConfig == nil {
		return false
	}
	return c.RestConfig.ExecProvider != nil || c.RestConfig.AuthProvider != nil
}
