package k8s_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// TestStartPodPortForwardUpgradeDenied pins what a denied SPDY upgrade looks
// like today: the session starts, never becomes ready, reports the
// apiserver's denial as text on Err (client-go formats it with %s, so the
// Forbidden type is gone) and then finishes on its own. The upgrade URL
// keeps any path prefix in the server address, as Rancher-proxied clusters
// need.
func TestStartPodPortForwardUpgradeDenied(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		prefix   string
		wantPath string
		wantErr  string
	}{
		{
			name: "plain server", wantPath: "/api/v1/namespaces/demo/pods/web/portforward",
			wantErr: `pods "web" is forbidden: User "` + k8stest.User + `" cannot create resource "pods/portforward" in API group "" in the namespace "demo"`,
		},
		{
			name: "proxied server", prefix: "/k8s/clusters/c-abc", wantPath: "/k8s/clusters/c-abc/api/v1/namespaces/demo/pods/web/portforward",
			wantErr: `forbidden: User "` + k8stest.User + `" cannot post path "/k8s/clusters/c-abc/api/v1/namespaces/demo/pods/web/portforward"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := &pathRecorder{next: k8stest.Forbidden()}
			base := k8stest.NewServerClient(t, rec)
			c := &k8s.Client{
				Clientset:  base.Clientset,
				Context:    base.Context,
				RestConfig: &rest.Config{Host: base.RestConfig.Host + tc.prefix},
			}

			sess, err := c.StartPodPortForward(context.Background(), "demo", "web", 0, 8080)
			if err != nil {
				t.Fatalf("StartPodPortForward: %v", err)
			}
			t.Cleanup(sess.Close)

			select {
			case err := <-sess.Err:
				if !strings.HasPrefix(err.Error(), "error upgrading connection: ") ||
					!strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Err = %q, want the upgrade denial", err)
				}
			case <-sess.Ready:
				t.Fatal("session became ready although the upgrade was denied")
			case <-time.After(10 * time.Second):
				t.Fatal("no error from a denied upgrade")
			}
			select {
			case <-sess.Done:
			case <-time.After(10 * time.Second):
				t.Fatal("session goroutine did not finish after the failed upgrade")
			}
			if sess.Pod != "web" {
				t.Fatalf("Pod = %q, want web", sess.Pod)
			}
			if paths := rec.seen(); len(paths) == 0 || slices.ContainsFunc(paths, func(p string) bool { return p != tc.wantPath }) {
				t.Fatalf("upgrade requests = %v, want [%s]", paths, tc.wantPath)
			}
		})
	}
}
