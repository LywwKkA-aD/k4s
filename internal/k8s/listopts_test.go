package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// TestListFunctionsTakeOneListOption pins what every list asks the
// apiserver for: a consistent read (ResourceVersion "") by default,
// the watch cache ("0") with FromCache.
func TestListFunctionsTakeOneListOption(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "demo"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d", Namespace: "demo"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "demo"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
	}
	lists := []struct {
		resource string
		list     func(context.Context, *Client, ...ListOption) (int, error)
	}{
		{"pods", func(ctx context.Context, c *Client, o ...ListOption) (int, error) {
			l, err := c.ListPods(ctx, "demo", o...)
			return len(l), err
		}},
		{"deployments", func(ctx context.Context, c *Client, o ...ListOption) (int, error) {
			l, err := c.ListDeployments(ctx, "demo", o...)
			return len(l), err
		}},
		{"services", func(ctx context.Context, c *Client, o ...ListOption) (int, error) {
			l, err := c.ListServices(ctx, "demo", o...)
			return len(l), err
		}},
		{"namespaces", func(ctx context.Context, c *Client, o ...ListOption) (int, error) {
			l, err := c.ListNamespaces(ctx, o...)
			return len(l), err
		}},
	}
	opts := []struct {
		name string
		opts []ListOption
		want string
	}{
		{name: "default is a consistent read", want: ""},
		{name: "FromCache reads the watch cache", opts: []ListOption{FromCache()}, want: "0"},
	}
	for _, l := range lists {
		for _, o := range opts {
			t.Run(l.resource+" "+o.name, func(t *testing.T) {
				t.Parallel()
				cs := fake.NewSimpleClientset(objs...)
				seen := make(chan string, 1)
				cs.PrependReactor("list", l.resource, func(a k8stesting.Action) (bool, runtime.Object, error) {
					la, ok := a.(interface{ GetListOptions() metav1.ListOptions })
					if !ok {
						t.Errorf("list action %T carries no ListOptions", a)
						return false, nil, nil
					}
					seen <- la.GetListOptions().ResourceVersion
					return false, nil, nil
				})
				n, err := l.list(context.Background(), &Client{Clientset: cs, Context: "test"}, o.opts...)
				if err != nil || n != 1 {
					t.Fatalf("list = %d, %v; want the one object", n, err)
				}
				if got := <-seen; got != o.want {
					t.Errorf("ResourceVersion = %q, want %q", got, o.want)
				}
			})
		}
	}
}
