package k8s

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStatsCountsResourcesAcrossNamespaces(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "demo"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p2", Namespace: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p3", Namespace: "demo"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s2", Namespace: "demo"}},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}

	got, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() returned error: %v", err)
	}

	want := Stats{Namespaces: 2, Pods: 3, Deployments: 1, Services: 2}
	if got != want {
		t.Errorf("Stats() = %+v, want %+v", got, want)
	}
}
