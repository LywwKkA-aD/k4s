package k8s_test

import (
	"context"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func TestListNamespaces(t *testing.T) {
	t.Parallel()

	created := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	c, _ := k8stest.NewFakeClient(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default", CreationTimestamp: created},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "leaving"},
			Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating},
		},
	)

	got, err := c.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	want := []k8s.Namespace{
		{Name: "default", Status: corev1.NamespaceActive, Age: created},
		{Name: "leaving", Status: corev1.NamespaceTerminating},
	}
	if !slices.EqualFunc(got, want, func(a, b k8s.Namespace) bool {
		return a.Name == b.Name && a.Status == b.Status && a.Age.Equal(&b.Age)
	}) {
		t.Fatalf("ListNamespaces = %+v, want %+v", got, want)
	}
}

// TestListNamespacesResourceVersion: a plain list is a consistent read
// (RV ""); FromCache serves it from the apiserver's watch cache (RV "0").
func TestListNamespacesResourceVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts []k8s.ListOption
		want string
	}{
		{name: "consistent by default", want: ""},
		{name: "from the watch cache", opts: []k8s.ListOption{k8s.FromCache()}, want: "0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}})
			seen := make(chan string, 1)
			cs.PrependReactor("list", "namespaces", func(a ktesting.Action) (bool, runtime.Object, error) {
				if l, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
					seen <- l.GetListOptions().ResourceVersion
				}
				return false, nil, nil
			})

			got, err := c.ListNamespaces(context.Background(), tc.opts...)
			if err != nil || len(got) != 1 {
				t.Fatalf("ListNamespaces = %+v, %v; want the one namespace", got, err)
			}
			select {
			case rv := <-seen:
				if rv != tc.want {
					t.Errorf("ResourceVersion = %q, want %q", rv, tc.want)
				}
			default:
				t.Fatal("the reactor saw no list")
			}
		})
	}
}
