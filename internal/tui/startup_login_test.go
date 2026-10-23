package tui_test

import (
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// unauthorized answers every request 401, as an apiserver does when the
// credential plugin produced no token.
var unauthorized = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"kind":"Status","apiVersion":"v1","status":"Failure","message":"Unauthorized","reason":"Unauthorized","code":401}`))
})

// loginClient is a context "eks" whose server refuses every request and
// counts the /version ones in versions; with plugin set its kubeconfig
// names a credential plugin.
func loginClient(t *testing.T, plugin bool, versions *atomic.Int32) *k8s.Client {
	t.Helper()
	c := k8stest.NewServerClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version" {
			versions.Add(1)
		}
		unauthorized.ServeHTTP(w, r)
	}))
	c.Context, c.CurrentContext = "eks", "eks"
	if plugin {
		c.RestConfig = &rest.Config{Host: c.RestConfig.Host, ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}
	}
	return c
}

// TestStartupOffersTheLoginAPreflightFailedFor: a context with a
// credential plugin whose preflight failed for want of a login offers the
// terminal login :ctx offers. The preflight's error is the answer, so the
// start asks no /version of its own: one more would run the plugin again,
// next to the first list, and could prompt or open a browser a second
// time. A start without a preflight (--no-preflight) is not probed; its
// lists say what is wrong. A context without a plugin asks /version once,
// for the header.
func TestStartupOffersTheLoginAPreflightFailedFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		plugin   bool
		session  startup.Session
		offered  bool
		versions int32
	}{
		{name: "no preflight", plugin: true},
		{
			name: "a preflight the plugin failed", plugin: true, offered: true,
			session: startup.Session{PreflightErr: errors.New("preflight: getting credentials: exec: executable aws failed with exit code 255")},
		},
		{
			name: "a preflight the server refused", plugin: true, offered: true,
			session: startup.Session{PreflightErr: fmt.Errorf("preflight: %w", apierrors.NewUnauthorized("Unauthorized"))},
		},
		{
			name: "a preflight that could not reach the server", plugin: true,
			session: startup.Session{PreflightErr: errors.New("preflight: dial tcp 10.0.0.1:6443: connect: connection refused")},
		},
		{
			name: "a preflight that logged in", plugin: true,
			session: startup.Session{ServerVersion: &apiversion.Info{GitVersion: "v1.30.4-eks-a737599"}},
		},
		{name: "no credential plugin", versions: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var versions atomic.Int32
			m, _ := tuitest.Mount(t, tui.New(loginClient(t, tc.plugin, &versions), nil, tui.WithSession(tc.session), instantFrames()), screenW, screenH)
			if got := versions.Load(); got != tc.versions {
				t.Errorf("/version asked %d times at the start, want %d", got, tc.versions)
			}
			if tc.offered {
				requireScreen(t, m, "log in", "Context eks needs a login")
				return
			}
			refuteScreen(t, m, "needs a login")
		})
	}
}
