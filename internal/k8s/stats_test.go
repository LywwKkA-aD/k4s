package k8s

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
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

	if got.Namespaces != 2 || got.Pods != 3 || got.Deployments != 1 || got.Services != 2 {
		t.Errorf("Stats() = %+v, want namespaces=2 pods=3 deployments=1 services=2", got)
	}
	if len(got.Failed) != 0 {
		t.Errorf("Stats().Failed = %v, want empty", got.Failed)
	}
}

func TestStatsPartialFailureKeepsOtherCounts(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "s1", Namespace: "default"}},
	}
	cs := fake.NewSimpleClientset(objs...)
	// Simulate an RBAC denial on services only — the other counts must
	// survive and services must land in Failed instead of erroring out.
	cs.PrependReactor("list", "services", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("forbidden")
	})
	c := &Client{Clientset: cs, Context: "test"}

	got, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() with one failing resource returned error: %v", err)
	}
	if got.Namespaces != 1 || got.Pods != 1 {
		t.Errorf("Stats() counts = %+v, want namespaces=1 pods=1", got)
	}
	if len(got.Failed) != 1 || got.Failed[0] != "services" {
		t.Errorf("Stats().Failed = %v, want [services]", got.Failed)
	}
}

func TestStatsReturnsErrorOnlyWhenEverythingFails(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "*", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	c := &Client{Clientset: cs, Context: "test"}

	got, err := c.Stats(context.Background())
	if err == nil {
		t.Fatal("Stats() with all resources failing returned nil error")
	}
	if len(got.Failed) != 4 {
		t.Errorf("Stats().Failed = %v, want all four resources", got.Failed)
	}
}
