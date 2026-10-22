package tui_test

import (
	"errors"
	"net/http"
	"testing"

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

// loginClient is a context "eks" whose server refuses every request; with
// plugin set its kubeconfig names a credential plugin.
func loginClient(t *testing.T, plugin bool) *k8s.Client {
	t.Helper()
	c := k8stest.NewServerClient(t, unauthorized)
	c.Context, c.CurrentContext = "eks", "eks"
	if plugin {
		c.RestConfig = &rest.Config{Host: c.RestConfig.Host, ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}
	}
	return c
}

// TestStartupProbesALoginThePreflightDidNotGet: a context with a
// credential plugin that started without a working preflight (none with
// --no-preflight, or one that failed) is probed as after :ctx, and a
// refused probe offers the terminal login.
func TestStartupProbesALoginThePreflightDidNotGet(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		plugin  bool
		session startup.Session
		offered bool
	}{
		{name: "no preflight", plugin: true, offered: true},
		{
			name: "a failed preflight", plugin: true, offered: true,
			session: startup.Session{PreflightErr: errors.New("preflight: getting credentials: exec: executable aws failed with exit code 255")},
		},
		{
			name: "a preflight that logged in", plugin: true,
			session: startup.Session{ServerVersion: &apiversion.Info{GitVersion: "v1.30.4-eks-a737599"}},
		},
		{name: "no credential plugin"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tui.New(loginClient(t, tc.plugin), nil, tui.WithSession(tc.session)), screenW, screenH)
			if tc.offered {
				requireScreen(t, m, "log in", "Context eks needs a login")
				return
			}
			refuteScreen(t, m, "needs a login")
		})
	}
}
