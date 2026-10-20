package k8s_test

import (
	"context"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
