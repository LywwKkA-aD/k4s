package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
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

func TestWarmUp(t *testing.T) {
	t.Parallel()

	t.Run("requests the version endpoint", func(t *testing.T) {
		t.Parallel()
		var hits atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/version" {
				hits.Add(1)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"gitVersion":"v1.31.4+k3s1"}`))
		}))
		t.Cleanup(srv.Close)

		if err := clientFor(t, srv.URL).WarmUp(context.Background()); err != nil {
			t.Fatalf("WarmUp: %v", err)
		}
		if hits.Load() != 1 {
			t.Fatalf("/version hits = %d, want 1", hits.Load())
		}
	})

	t.Run("server error is returned", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)

		if err := clientFor(t, srv.URL).WarmUp(context.Background()); err == nil {
			t.Fatal("WarmUp succeeded against a failing server")
		}
	})

	t.Run("honours the context deadline", func(t *testing.T) {
		t.Parallel()
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			select {
			case <-release:
			case <-r.Context().Done():
			}
		}))
		t.Cleanup(func() { close(release); srv.Close() })

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := clientFor(t, srv.URL).WarmUp(ctx); err == nil {
			t.Fatal("WarmUp succeeded against a hanging server")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("WarmUp ignored the deadline: took %v", elapsed)
		}
	})

	t.Run("fake clientset is a no-op", func(t *testing.T) {
		t.Parallel()
		c := &Client{Clientset: fake.NewSimpleClientset()}
		if err := c.WarmUp(context.Background()); err != nil {
			t.Fatalf("WarmUp on a fake clientset: %v", err)
		}
	})
}
