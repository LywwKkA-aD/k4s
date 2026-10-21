package startup

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/cli"
	"github.com/LywwKkA-aD/k4s/internal/cluster"
)

const pluginKubeconfig = `apiVersion: v1
kind: Config
current-context: sso
clusters:
- name: c
  cluster: {server: %s, certificate-authority-data: %s}
contexts:
- name: sso
  context: {cluster: c, user: plugin}
users:
- name: plugin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: %s
      interactiveMode: IfAvailable
`

// pluginScript is a credential plugin that sleeps, then prints a token or
// fails.
const pluginScript = `#!/bin/sh
sleep %s
%s
printf '{"apiVersion":"client.authentication.k8s.io/v1","kind":"ExecCredential","status":{"token":"t"}}\n'
`

// versionServer answers /version after delay. requested, when not nil,
// is closed on the first request, which a plugin context only sends once
// its plugin has finished.
func versionServer(t *testing.T, delay time.Duration, hits *atomic.Int32, requested chan struct{}) *httptest.Server {
	t.Helper()
	var once sync.Once
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if requested != nil {
			once.Do(func() { close(requested) })
		}
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.5+k3s1"}`))
	}))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	return srv
}

// pluginSession opens a session on srv whose context authenticates through
// a plugin that sleeps, then runs fail ("" succeeds). client-go only sends
// credentials over TLS, hence the TLS server and its CA.
func pluginSession(t *testing.T, srv *httptest.Server, sleep, fail string) Session {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the test plugin is a shell script")
	}
	dir := t.TempDir()
	plugin := write(t, filepath.Join(dir, "plugin.sh"), fmt.Sprintf(pluginScript, sleep, fail))
	chmodExec(t, plugin)
	ca := base64.StdEncoding.EncodeToString(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	kc := write(t, filepath.Join(dir, "config"), fmt.Sprintf(pluginKubeconfig, srv.URL, ca, plugin))
	s, err := Open(cli.Options{Kubeconfig: kc}, newMachine(t).Env())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !s.Client.NeedsLogin() {
		t.Fatal("plugin context does not need a login")
	}
	return s
}

func chmodExec(t *testing.T, path string) {
	t.Helper()
	if err := os.Chmod(path, 0o700); err != nil { //nolint:gosec // a plugin must be executable
		t.Fatal(err)
	}
}

func hostPort(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func TestPreflightSkipsPlainCredentials(t *testing.T) {
	t.Parallel()

	var hits atomic.Int32
	srv := versionServer(t, 0, &hits, nil)
	m := newMachine(t)
	write(t, m.homeConfig(), strings.Replace(kubeconfigBody, "https://127.0.0.1:6443", srv.URL, 1))
	s, err := Open(cli.Options{}, m.Env())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	var out bytes.Buffer
	got, err := Preflight(context.Background(), s, PreflightConfig{Out: &out})
	if err != nil || got.ServerVersion != nil || got.PreflightErr != nil {
		t.Fatalf("Preflight = %+v, %v; want the session untouched", got, err)
	}
	if hits.Load() != 0 || out.Len() != 0 {
		t.Fatalf("plain credentials made %d requests and printed %q", hits.Load(), out.String())
	}
	if _, err := Preflight(context.Background(), Session{}, PreflightConfig{}); err != nil {
		t.Fatalf("Preflight without a client: %v", err)
	}
}

// preflightRun is what one Preflight call produced.
type preflightRun struct {
	s    Session
	err  error
	out  string
	host string
}

func checkFastPreflight(t *testing.T, r preflightRun) {
	t.Helper()
	if r.err != nil || r.s.PreflightErr != nil {
		t.Fatalf("Preflight = %v / %v", r.err, r.s.PreflightErr)
	}
	if v := r.s.ServerVersion; v == nil || v.GitVersion != "v1.36.5+k3s1" || r.s.Distro != cluster.K3s {
		t.Errorf("ServerVersion/Distro = %+v/%q", v, r.s.Distro)
	}
	if r.out != "" {
		t.Errorf("a fast preflight printed %q", r.out)
	}
	if !r.s.Client.RestConfig.ExecProvider.StdinUnavailable {
		t.Error("the runtime client was swapped for the interactive one")
	}
}

func checkNotice(t *testing.T, r preflightRun) {
	t.Helper()
	if r.err != nil || r.s.ServerVersion == nil {
		t.Fatalf("Preflight = %+v, %v", r.s, r.err)
	}
	if want := "k4s: connecting to sso (" + r.host + ")…\n"; r.out != want {
		t.Errorf("notice = %q, want %q", r.out, want)
	}
}

func checkNoAnswer(t *testing.T, r preflightRun) {
	t.Helper()
	if r.err != nil {
		t.Fatalf("Preflight err = %v, want the TUI to start", r.err)
	}
	want := "preflight: no answer from " + r.host + " within 150ms"
	if r.s.PreflightErr == nil || !strings.Contains(r.s.PreflightErr.Error(), want) {
		t.Errorf("PreflightErr = %v, want %q", r.s.PreflightErr, want)
	}
}

func checkPluginFailed(t *testing.T, r preflightRun) {
	t.Helper()
	if r.err != nil || r.s.PreflightErr == nil || r.s.ServerVersion != nil {
		t.Fatalf("Preflight = %+v, %v; want PreflightErr", r.s, r.err)
	}
}

func checkPluginStuck(t *testing.T, r preflightRun) {
	t.Helper()
	if !errors.Is(r.err, errPluginStuck) || !strings.Contains(r.err.Error(), `context "sso"`) {
		t.Fatalf("Preflight err = %v, want errPluginStuck naming the context", r.err)
	}
}

func checkAborted(t *testing.T, r preflightRun) {
	t.Helper()
	if !errors.Is(r.err, ErrAborted) {
		t.Fatalf("Preflight err = %v, want ErrAborted", r.err)
	}
}

func TestPreflight(t *testing.T) {
	t.Parallel()

	// The first exec of a fresh script can take seconds on macOS, so only
	// the stuck-plugin case relies on a short Grace.
	quiet, patient := time.Minute, time.Minute
	cases := []struct {
		name        string
		serverDelay time.Duration
		sleep, fail string
		cfg         PreflightConfig
		// cancelOnRequest presses Ctrl+C once the plugin is done and the
		// server holds the request.
		cancelOnRequest bool
		check           func(*testing.T, preflightRun)
	}{
		{name: "fast plugin and server", cfg: PreflightConfig{NoticeAfter: quiet}, check: checkFastPreflight},
		{
			name:        "slow answer prints what it waits for",
			serverDelay: 300 * time.Millisecond,
			cfg:         PreflightConfig{NoticeAfter: 20 * time.Millisecond},
			check:       checkNotice,
		},
		{
			name:        "server never answers",
			serverDelay: time.Hour,
			cfg:         PreflightConfig{Timeout: 150 * time.Millisecond, Grace: patient, NoticeAfter: quiet},
			check:       checkNoAnswer,
		},
		{name: "plugin fails", fail: "exit 1", cfg: PreflightConfig{NoticeAfter: quiet}, check: checkPluginFailed},
		{
			name:  "plugin still running after the timeout",
			sleep: "3",
			cfg:   PreflightConfig{Timeout: 100 * time.Millisecond, Grace: 100 * time.Millisecond, NoticeAfter: quiet},
			check: checkPluginStuck,
		},
		{
			name:            "ctrl+c aborts",
			serverDelay:     time.Hour,
			cancelOnRequest: true,
			cfg:             PreflightConfig{NoticeAfter: quiet},
			check:           checkAborted,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var hits atomic.Int32
			var requested chan struct{}
			if tc.cancelOnRequest {
				requested = make(chan struct{})
			}
			srv := versionServer(t, tc.serverDelay, &hits, requested)
			sleep := tc.sleep
			if sleep == "" {
				sleep = "0"
			}
			s := pluginSession(t, srv, sleep, tc.fail)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if requested != nil {
				go func() {
					<-requested
					cancel()
				}()
			}
			var out bytes.Buffer
			cfg := tc.cfg
			cfg.Out = &out
			got, err := Preflight(ctx, s, cfg)
			tc.check(t, preflightRun{s: got, err: err, out: out.String(), host: hostPort(t, srv)})
		})
	}
}

func TestHostOf(t *testing.T) {
	t.Parallel()

	withUser := url.URL{Scheme: "https", User: url.UserPassword("admin", "pw"), Host: "10.0.0.5:6443", Path: "/prefix"}
	cases := []struct {
		host string
		want string
	}{
		{host: "https://10.0.0.5:6443", want: "10.0.0.5:6443"},
		{host: withUser.String(), want: "10.0.0.5:6443"},
		{host: "127.0.0.1:6443", want: "127.0.0.1:6443"},
		{host: "", want: ""},
		{host: "https://[::1", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			t.Parallel()
			if got := hostOf(&rest.Config{Host: tc.host}); got != tc.want {
				t.Fatalf("hostOf(%q) = %q, want %q", tc.host, got, tc.want)
			}
		})
	}
	if hostOf(nil) != "" {
		t.Fatal("hostOf(nil) is not empty")
	}
}
