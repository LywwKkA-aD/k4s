package k8stest

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// ContextName is the kubeconfig context the clients built here report.
const ContextName = "k8stest"

// NewFakeClient returns a *k8s.Client over fake.NewSimpleClientset(objs...)
// together with that clientset, so a test can install reactors such as
// RestrictToNamespace before handing the client to the code under test.
//
// A fake clientset has no REST client behind Discovery(), so raw REST calls
// (ListPodMetrics, ListNodeMetrics) panic on it; use NewServerClient there.
func NewFakeClient(objs ...runtime.Object) (*k8s.Client, *fake.Clientset) {
	cs := fake.NewSimpleClientset(objs...)
	return &k8s.Client{Clientset: cs, Context: ContextName}, cs
}

// NewServerClient serves h from an httptest server for the life of t and
// returns a *k8s.Client whose real clientset talks to it, built from
// rest.Config{Host: server URL}. Errors therefore go through client-go's
// own decoding, and the caller's context deadline really cuts the request.
func NewServerClient(t testing.TB, h http.Handler) *k8s.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	// Hang keeps requests open until the client leaves; dropping the
	// connections first stops Close from waiting on them.
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	cfg := &rest.Config{Host: srv.URL}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("build clientset for %s: %v", srv.URL, err)
	}
	return &k8s.Client{Clientset: cs, Context: ContextName, RestConfig: cfg}
}
