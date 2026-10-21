package k8s_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// lister runs one of the client's List calls and reports how many items
// came back.
type lister struct {
	name   string
	prefix string // the wrap the error must carry (C-25)
	list   func(ctx context.Context, c *k8s.Client, ns string) (int, error)
}

func listers() []lister {
	return []lister{
		{name: "pods", prefix: "list pods: ", list: func(ctx context.Context, c *k8s.Client, ns string) (int, error) {
			items, err := c.ListPods(ctx, ns)
			return len(items), err
		}},
		{name: "deployments", prefix: "list deployments: ", list: func(ctx context.Context, c *k8s.Client, ns string) (int, error) {
			items, err := c.ListDeployments(ctx, ns)
			return len(items), err
		}},
		{name: "services", prefix: "list services: ", list: func(ctx context.Context, c *k8s.Client, ns string) (int, error) {
			items, err := c.ListServices(ctx, ns)
			return len(items), err
		}},
		{name: "namespaces", prefix: "list namespaces: ", list: func(ctx context.Context, c *k8s.Client, _ string) (int, error) {
			items, err := c.ListNamespaces(ctx)
			return len(items), err
		}},
	}
}

func TestListsUnderNamespaceScopedRBAC(t *testing.T) {
	t.Parallel()

	scopes := []struct {
		name      string
		namespace string
		wantScope string // empty: allowed
	}{
		{name: "granted namespace", namespace: granted},
		{name: "all namespaces", namespace: "", wantScope: "at the cluster scope"},
		{name: "other namespace", namespace: "other", wantScope: `in the namespace "other"`},
	}
	for _, l := range listers() {
		for _, s := range scopes {
			if l.name == "namespaces" && s.namespace != "" {
				continue // cluster-scoped: a Role can never grant it
			}
			t.Run(l.name+"/"+s.name, func(t *testing.T) {
				t.Parallel()
				c, cs := k8stest.NewFakeClient(clusterObjects()...)
				k8stest.RestrictToNamespace(cs, granted)

				n, err := l.list(context.Background(), c, s.namespace)
				if s.wantScope == "" {
					if err != nil || n != 1 {
						t.Fatalf("got %d items, err %v; want the one %s object", n, err, granted)
					}
					return
				}
				assertForbiddenList(t, err, l.prefix, s.wantScope)
				if n != 0 {
					t.Fatalf("got %d items alongside the error, want 0", n)
				}
			})
		}
	}
}

func assertForbiddenList(t *testing.T, err error, prefix, scope string) {
	t.Helper()
	if !apierrors.IsForbidden(err) {
		t.Fatalf("err = %v, want Forbidden reachable through the wrap", err)
	}
	var status *apierrors.StatusError
	if !errors.As(err, &status) || status.Status().Code != 403 {
		t.Fatalf("err = %v, want a 403 *StatusError inside", err)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, prefix) {
		t.Fatalf("err = %q, want prefix %q", msg, prefix)
	}
	if !strings.Contains(msg, "is forbidden: User \""+k8stest.User+"\"") || !strings.HasSuffix(msg, scope) {
		t.Fatalf("err = %q, want the apiserver's denial ending %q", msg, scope)
	}
}

func TestListsWithRevokedCredentials(t *testing.T) {
	t.Parallel()

	for _, l := range listers() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(clusterObjects()...)
			k8stest.Unauthorized(cs)

			_, err := l.list(context.Background(), c, granted)
			if !apierrors.IsUnauthorized(err) {
				t.Fatalf("err = %v, want Unauthorized reachable through the wrap", err)
			}
			if want := l.prefix + "Unauthorized"; err.Error() != want {
				t.Fatalf("err = %q, want %q", err, want)
			}
		})
	}
}

// The fake clientset drops the context, so this only proves the list
// wrappers keep a deadline cause reachable through %w; real deadlines are
// covered over REST with k8stest.Hang().
func TestListErrorsKeepDeadlineCause(t *testing.T) {
	t.Parallel()

	for _, l := range listers() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(clusterObjects()...)
			k8stest.SlowerThanDeadline(cs, 20*time.Millisecond)

			_, err := l.list(context.Background(), c, "")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want context.DeadlineExceeded reachable through the wrap", err)
			}
			if !strings.HasPrefix(err.Error(), l.prefix) {
				t.Fatalf("err = %q, want prefix %q", err, l.prefix)
			}
		})
	}
}

func TestListsHonourTheCallerDeadlineOverREST(t *testing.T) {
	t.Parallel()

	for _, l := range listers() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Hang())
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()

			start := time.Now()
			_, err := l.list(ctx, c, granted)
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want context.DeadlineExceeded", err)
			}
			if !strings.HasPrefix(err.Error(), l.prefix) {
				t.Fatalf("err = %q, want prefix %q", err, l.prefix)
			}
			if elapsed := time.Since(start); elapsed > 5*time.Second {
				t.Fatalf("list outlived its deadline: %v", elapsed)
			}
		})
	}
}

func TestListsDecodeServerForbiddenOverREST(t *testing.T) {
	t.Parallel()

	for _, l := range listers() {
		t.Run(l.name, func(t *testing.T) {
			t.Parallel()
			c := k8stest.NewServerClient(t, k8stest.Forbidden())

			_, err := l.list(context.Background(), c, "")
			assertForbiddenList(t, err, l.prefix, "at the cluster scope")
		})
	}
}
