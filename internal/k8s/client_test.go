package k8s

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const testKubeconfig = `apiVersion: v1
kind: Config
current-context: %s
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: alpha
  context: {cluster: c1, user: u1, namespace: team-a}
- name: beta
  context: {cluster: c1, user: u1}
users:
- name: u1
  user: {token: not-a-real-token}
`

func writeKubeconfig(t *testing.T, current string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	body := strings.Replace(testKubeconfig, "%s", current, 1)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFromKubeconfig(t *testing.T) {
	t.Parallel()

	path := writeKubeconfig(t, "alpha")
	c, err := LoadFromKubeconfig(path)
	if err != nil {
		t.Fatalf("LoadFromKubeconfig: %v", err)
	}
	if c.Context != "alpha" {
		t.Errorf("Context = %q, want alpha", c.Context)
	}
	if !slices.Equal(c.KubeconfigPaths, []string{path}) {
		t.Errorf("KubeconfigPaths = %v, want [%s]", c.KubeconfigPaths, path)
	}
	if c.RestConfig == nil || c.RestConfig.Host != "https://127.0.0.1:6443" {
		t.Errorf("RestConfig not built from the kubeconfig: %+v", c.RestConfig)
	}
}

func TestLoadFromKubeconfigContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		ctx     string
		want    string
		wantErr string
	}{
		{name: "explicit context wins", current: "alpha", ctx: "beta", want: "beta"},
		{name: "empty falls back to current", current: "alpha", ctx: "", want: "alpha"},
		{name: "unknown context", current: "alpha", ctx: "gamma", wantErr: `context "gamma" not found`},
		{name: "no current context", current: `""`, ctx: "", wantErr: "no current-context"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := writeKubeconfig(t, tc.current)
			c, err := LoadFromKubeconfigContext(path, tc.ctx)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadFromKubeconfigContext: %v", err)
			}
			if c.Context != tc.want {
				t.Errorf("Context = %q, want %q", c.Context, tc.want)
			}
			if !slices.Equal(c.KubeconfigPaths, []string{path}) {
				t.Errorf("KubeconfigPaths = %v, want [%s]", c.KubeconfigPaths, path)
			}
		})
	}
}
