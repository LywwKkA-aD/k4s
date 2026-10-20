package logsink_test

import (
	"bytes"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/logsink"
)

const pluginMarker = "open https://login.example/device and enter ABCD-EFGH"

const pluginScript = `#!/bin/sh
echo "` + pluginMarker + `" >&2
cat <<'EOF'
{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"plugin-token"}}
EOF
`

const execKubeconfig = `apiVersion: v1
kind: Config
current-context: exec
clusters:
- name: c1
  cluster:
    server: %q
    certificate-authority-data: %s
contexts:
- name: exec
  context: {cluster: c1, user: plugin}
users:
- name: plugin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: %q
      interactiveMode: Never
`

// The exec plugin only runs on the first API request, long after the
// clientset is built. This proves the stderr client-go captures at build
// time is the in-memory pipe, so the plugin's device-code prompt neither
// paints over the TUI nor lands in the log file. Swaps os.Stderr, so it
// does not run in parallel.
func TestExecPluginStderrIsHeldUntilFlush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script exec plugin")
	}

	var sawToken atomic.Bool
	// clientcmd only applies user credentials over TLS.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer plugin-token" {
			sawToken.Store(true)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"major":"1","minor":"31","gitVersion":"v1.31.4"}`)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	plugin := filepath.Join(dir, "plugin.sh")
	if err := os.WriteFile(plugin, []byte(pluginScript), 0o700); err != nil { //nolint:gosec // an exec plugin must be executable
		t.Fatal(err)
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	body := fmt.Appendf(nil, execKubeconfig, srv.URL, base64.StdEncoding.EncodeToString(ca), plugin)
	kubeconfig := filepath.Join(dir, "config")
	if err := os.WriteFile(kubeconfig, body, 0o600); err != nil {
		t.Fatal(err)
	}

	stop := logsink.SwapStderr(t)
	logsink.EnablePluginCapture()
	logsink.HoldPluginStderr()
	t.Cleanup(func() { _ = logsink.FlushPluginStderr(io.Discard) })

	client, err := k8s.LoadFromKubeconfigContext(kubeconfig, "")
	if err != nil {
		t.Fatalf("LoadFromKubeconfigContext: %v", err)
	}
	if _, err := client.Clientset.Discovery().ServerVersion(); err != nil {
		t.Fatalf("ServerVersion: %v", err)
	}
	if !sawToken.Load() {
		t.Fatal("request did not carry the plugin's token: plugin never ran")
	}

	var held bytes.Buffer
	if err := logsink.FlushPluginStderr(&held); err != nil {
		t.Fatalf("FlushPluginStderr: %v", err)
	}
	terminal := stop()

	if !strings.Contains(held.String(), pluginMarker) {
		t.Errorf("flushed plugin output %q missing %q", held.String(), pluginMarker)
	}
	if strings.Contains(terminal, pluginMarker) {
		t.Errorf("plugin output reached stderr while capture was on: %q", terminal)
	}
}
