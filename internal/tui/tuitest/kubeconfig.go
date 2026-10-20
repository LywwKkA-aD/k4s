package tuitest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UnreachableServer is the API server every WriteKubeconfig context points
// at. Port 1 on loopback refuses connections at once, so a client built
// from it fails fast instead of waiting for a timeout.
const UnreachableServer = "https://127.0.0.1:1"

// WriteKubeconfig writes dir/config with one context per name, all on
// UnreachableServer with a placeholder token, and current as the
// current-context. It returns the file path; point KUBECONFIG at it.
func WriteKubeconfig(dir, current string, names ...string) (string, error) {
	var b strings.Builder
	b.WriteString("apiVersion: v1\nkind: Config\n")
	fmt.Fprintf(&b, "current-context: %q\n", current)
	b.WriteString("clusters:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s-cluster\n  cluster:\n    server: %s\n", n, UnreachableServer)
	}
	b.WriteString("users:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s-user\n  user:\n    token: placeholder\n", n)
	}
	b.WriteString("contexts:\n")
	for _, n := range names {
		fmt.Fprintf(&b, "- name: %s\n  context:\n    cluster: %s-cluster\n    user: %s-user\n    namespace: %s\n", n, n, n, NamespaceDemo)
	}
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return "", fmt.Errorf("write kubeconfig: %w", err)
	}
	return path, nil
}
