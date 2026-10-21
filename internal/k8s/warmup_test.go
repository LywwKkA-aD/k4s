package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func TestNeedsLogin(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		client *Client
		want   bool
	}{
		{name: "nil client", client: nil, want: false},
		{name: "no rest config", client: &Client{}, want: false},
		{name: "static token", client: &Client{RestConfig: &rest.Config{BearerToken: "t"}}, want: false},
		{name: "exec plugin", client: &Client{RestConfig: &rest.Config{ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}}, want: true},
		{name: "auth provider", client: &Client{RestConfig: &rest.Config{AuthProvider: &clientcmdapi.AuthProviderConfig{Name: "oidc"}}}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.client.NeedsLogin(); got != tc.want {
				t.Fatalf("NeedsLogin() = %v, want %v", got, tc.want)
			}
		})
	}
}

const plainKubeconfig = `apiVersion: v1
kind: Config
current-context: c
clusters:
- name: c
  cluster: {server: %s}
contexts:
- name: c
  context: {cluster: c, user: u}
users:
- name: u
  user: {token: not-a-real-token}
`

func clientFor(t *testing.T, server string) *Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, fmt.Appendf(nil, plainKubeconfig, server), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadFromKubeconfigContext(path, "")
	if err != nil {
		t.Fatalf("LoadFromKubeconfigContext: %v", err)
	}
	return c
}
