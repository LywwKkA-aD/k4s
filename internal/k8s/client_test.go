package k8s

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/version"
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

const execKubeconfig = `apiVersion: v1
kind: Config
current-context: sso
clusters:
- name: c1
  cluster:
    server: https://127.0.0.1:6443
contexts:
- name: sso
  context: {cluster: c1, user: plugin}
users:
- name: plugin
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      command: /nonexistent/credential-plugin
      interactiveMode: IfAvailable
`

func writeKubeconfig(t *testing.T, current string) string {
	t.Helper()
	return writeFile(t, "config", strings.Replace(testKubeconfig, "%s", current, 1))
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
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
		t.Fatalf("RestConfig not built from the kubeconfig: %+v", c.RestConfig)
	}
	if c.RestConfig.QPS != 50 || c.RestConfig.Burst != 100 {
		t.Errorf("QPS/Burst = %v/%d, want 50/100", c.RestConfig.QPS, c.RestConfig.Burst)
	}
	if want := "k4s/" + version.String(); c.RestConfig.UserAgent != want {
		t.Errorf("UserAgent = %q, want %q", c.RestConfig.UserAgent, want)
	}
	if qps := c.Clientset.CoreV1().RESTClient().GetRateLimiter().QPS(); qps != 50 {
		t.Errorf("clientset rate limiter QPS = %v, want 50", qps)
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

func TestLoadSources(t *testing.T) {
	t.Parallel()

	path := writeKubeconfig(t, "alpha")
	missing := filepath.Join(t.TempDir(), "absent")
	cases := []struct {
		name      string
		src       Source
		wantPaths []string
		wantArg   string
		wantErr   string
	}{
		{name: "flag", src: Source{Paths: []string{path}, Origin: OriginFlag}, wantPaths: []string{path}, wantArg: path},
		{name: "k3s fallback", src: Source{Paths: []string{path}, Origin: OriginK3s}, wantPaths: []string{path}, wantArg: path},
		{
			name:      "kubectl list skips missing files",
			src:       Source{Paths: []string{missing, path}, Origin: OriginKubectl},
			wantPaths: []string{missing, path},
		},
		{name: "missing flag file", src: Source{Paths: []string{missing}, Origin: OriginFlag}, wantErr: "load kubeconfig: "},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, err := Load(tc.src, "")
			if tc.wantErr != "" {
				if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) {
					t.Fatalf("Load err = %v, want prefix %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.Context != "alpha" {
				t.Errorf("Context = %q, want alpha", c.Context)
			}
			if !slices.Equal(c.KubeconfigPaths, tc.wantPaths) {
				t.Errorf("KubeconfigPaths = %v, want %v", c.KubeconfigPaths, tc.wantPaths)
			}
			if got := c.KubeconfigArg(); got != tc.wantArg {
				t.Errorf("KubeconfigArg() = %q, want %q", got, tc.wantArg)
			}
			if c.Source.Origin != tc.src.Origin || !slices.Equal(c.Source.Paths, tc.src.Paths) {
				t.Errorf("Source = %+v, want %+v", c.Source, tc.src)
			}
		})
	}
}

// TestLoadRecordsTheCurrentContext: CurrentContext is the file's
// current-context whichever context was loaded, so a footer command can
// tell when it needs --context.
func TestLoadRecordsTheCurrentContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		current string
		load    string
		want    string
	}{
		{name: "current context", current: "alpha", want: "alpha"},
		{name: "another context", current: "alpha", load: "beta", want: "alpha"},
		{name: "file without one", current: `""`, load: "beta", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := Source{Paths: []string{writeKubeconfig(t, tc.current)}, Origin: OriginFlag}
			c, err := Load(src, tc.load)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.CurrentContext != tc.want {
				t.Errorf("CurrentContext = %q, want %q", c.CurrentContext, tc.want)
			}
			next, err := c.SwitchContext("beta")
			if err != nil {
				t.Fatalf("SwitchContext: %v", err)
			}
			if next.CurrentContext != tc.want {
				t.Errorf("CurrentContext after SwitchContext = %q, want %q", next.CurrentContext, tc.want)
			}
		})
	}
}

func TestLoadNamespaceFromContext(t *testing.T) {
	t.Parallel()

	src := Source{Paths: []string{writeKubeconfig(t, "alpha")}, Origin: OriginFlag}
	cases := []struct {
		context string
		want    string
	}{
		{context: "alpha", want: "team-a"},
		{context: "beta", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.context, func(t *testing.T) {
			t.Parallel()
			c, err := Load(src, tc.context)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.ContextNamespace != tc.want || c.Namespace != tc.want {
				t.Errorf("ContextNamespace/Namespace = %q/%q, want %q", c.ContextNamespace, c.Namespace, tc.want)
			}
		})
	}
}

func TestLoadRejectsInvalidContextNamespace(t *testing.T) {
	t.Parallel()

	body := strings.Replace(strings.Replace(testKubeconfig, "%s", "alpha", 1), "namespace: team-a", "namespace: Team_A", 1)
	_, err := Load(Source{Paths: []string{writeFile(t, "config", body)}, Origin: OriginFlag}, "")
	if !errors.Is(err, ErrInvalidNamespace) || !strings.Contains(err.Error(), `context "alpha"`) {
		t.Fatalf("Load err = %v, want ErrInvalidNamespace naming the context", err)
	}
}

func TestSwitchContextKeepsTheSource(t *testing.T) {
	t.Parallel()

	src := Source{Paths: []string{writeKubeconfig(t, "alpha")}, Origin: OriginK3s}
	c, err := Load(src, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	next, err := c.WithNamespace("web").SwitchContext("beta")
	if err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}
	if next.Context != "beta" || next.Namespace != "" || next.KubeconfigArg() != src.Paths[0] {
		t.Fatalf("SwitchContext = ctx %q ns %q arg %q, want beta, all namespaces, %s",
			next.Context, next.Namespace, next.KubeconfigArg(), src.Paths[0])
	}
	if _, err := c.SwitchContext("gamma"); err == nil {
		t.Fatal("SwitchContext to an unknown context succeeded")
	}
}

func TestWithNamespaceCopies(t *testing.T) {
	t.Parallel()

	c := &Client{Context: "alpha", ContextNamespace: "team-a", Namespace: "team-a"}
	got := c.WithNamespace("")
	if got == c || got.Namespace != "" || got.ContextNamespace != "team-a" || got.Context != "alpha" {
		t.Fatalf("WithNamespace(\"\") = %+v", got)
	}
	if c.Namespace != "team-a" {
		t.Fatalf("WithNamespace mutated its receiver: %+v", c)
	}
}

func TestNilClientHelpers(t *testing.T) {
	t.Parallel()

	var c *Client
	if c.KubeconfigArg() != "" {
		t.Error("nil client has a kubeconfig arg")
	}
	// Without a client there is no Source: the default lookup is used. It
	// must fail with a load error, not panic.
	if _, err := c.SwitchContext("no-such-context-anywhere"); err == nil {
		t.Error("nil SwitchContext to an unknown context succeeded")
	}
	if got, err := c.Interactive(); got != nil || err != nil {
		t.Errorf("nil Interactive() = %v, %v", got, err)
	}
}

func TestRuntimeCredentialsNeverReadStdin(t *testing.T) {
	t.Parallel()

	path := writeFile(t, "config", execKubeconfig)
	c, err := Load(Source{Paths: []string{path}, Origin: OriginFlag}, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	ep := c.RestConfig.ExecProvider
	if ep == nil || !ep.StdinUnavailable {
		t.Fatalf("runtime ExecProvider = %+v, want StdinUnavailable", ep)
	}
	wantHint := "kubectl --context sso --kubeconfig " + path + " get ns"
	if !strings.Contains(ep.StdinUnavailableMessage, wantHint) {
		t.Errorf("StdinUnavailableMessage = %q, want it to contain %q", ep.StdinUnavailableMessage, wantHint)
	}

	login, err := c.Interactive()
	if err != nil {
		t.Fatalf("Interactive: %v", err)
	}
	if login == c || login.RestConfig.ExecProvider.StdinUnavailable || login.Clientset == c.Clientset {
		t.Fatalf("Interactive() = %+v, want a copy with its own clientset and stdin", login.RestConfig.ExecProvider)
	}
	if !c.RestConfig.ExecProvider.StdinUnavailable {
		t.Fatal("Interactive changed the runtime config")
	}
}

func TestInteractiveWithoutPluginIsTheSameClient(t *testing.T) {
	t.Parallel()

	c, err := Load(Source{Paths: []string{writeKubeconfig(t, "alpha")}, Origin: OriginFlag}, "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, err := c.Interactive(); got != c || err != nil {
		t.Fatalf("Interactive() = %p, %v; want the receiver %p", got, err, c)
	}
}

func TestServerVersion(t *testing.T) {
	t.Parallel()

	t.Run("decodes the version endpoint", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/version" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"major":"1","minor":"36","gitVersion":"v1.36.5+k3s1"}`))
		}))
		t.Cleanup(srv.Close)

		info, err := clientFor(t, srv.URL).ServerVersion(context.Background())
		if err != nil {
			t.Fatalf("ServerVersion: %v", err)
		}
		if info.GitVersion != "v1.36.5+k3s1" || info.Minor != "36" {
			t.Fatalf("ServerVersion = %+v", info)
		}
	})

	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantErr string
	}{
		{
			name:    "server error",
			handler: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) },
			wantErr: "get server version: ",
		},
		{
			name: "not json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`<html>`))
			},
			wantErr: "decode server version: ",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(tc.handler)
			t.Cleanup(srv.Close)
			_, err := clientFor(t, srv.URL).ServerVersion(context.Background())
			if err == nil || !strings.HasPrefix(err.Error(), tc.wantErr) {
				t.Fatalf("ServerVersion err = %v, want prefix %q", err, tc.wantErr)
			}
		})
	}

	t.Run("fake clientset", func(t *testing.T) {
		t.Parallel()
		cs := fake.NewSimpleClientset()
		cs.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &apiversion.Info{GitVersion: "v1.33.0"}
		info, err := (&Client{Clientset: cs}).ServerVersion(context.Background())
		if err != nil || info.GitVersion != "v1.33.0" {
			t.Fatalf("ServerVersion = %+v, %v", info, err)
		}
	})
}
