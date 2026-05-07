package k8s

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func intPtr(i int32) *int32 { return &i }

func TestListDeploymentsRendersStatusAndAge(t *testing.T) {
	t.Parallel()

	twoMinAgo := metav1.NewTime(time.Now().Add(-2 * time.Minute))

	objs := []runtime.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec:       appsv1.DeploymentSpec{Replicas: intPtr(3)},
			Status: appsv1.DeploymentStatus{
				Replicas:          3,
				ReadyReplicas:     2,
				UpdatedReplicas:   3,
				AvailableReplicas: 2,
			},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "spammer", Namespace: "demo", CreationTimestamp: twoMinAgo},
			Spec:       appsv1.DeploymentSpec{Replicas: intPtr(2)},
			Status: appsv1.DeploymentStatus{
				Replicas:          2,
				ReadyReplicas:     2,
				UpdatedReplicas:   2,
				AvailableReplicas: 2,
			},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}

	got, err := c.ListDeployments(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 deployments, got %d", len(got))
	}

	by := map[string]Deployment{}
	for _, d := range got {
		by[d.Name] = d
	}
	if d := by["nginx"]; d.Replicas != 3 || d.Ready != 2 || d.UpToDate != 3 || d.Available != 2 {
		t.Errorf("nginx counts wrong: %+v", d)
	}
	if d := by["spammer"]; d.Replicas != 2 || d.Ready != 2 {
		t.Errorf("spammer counts wrong: %+v", d)
	}
}

func TestListDeploymentsHandlesNilReplicas(t *testing.T) {
	t.Parallel()

	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec:       appsv1.DeploymentSpec{Replicas: nil},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(d)), Context: "test"}

	got, err := c.ListDeployments(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListDeployments: %v", err)
	}
	if len(got) != 1 || got[0].Replicas != 0 {
		t.Errorf("expected nil Spec.Replicas to surface as 0, got %+v", got)
	}
}

func TestPodsForDeploymentMatchesLabelSelector(t *testing.T) {
	t.Parallel()

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Replicas: intPtr(3),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
		},
	}
	objs := []runtime.Object{
		runtime.Object(dep),
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-a", Namespace: "demo", Labels: map[string]string{"app": "nginx"}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "nginx-b", Namespace: "demo", Labels: map[string]string{"app": "nginx"}},
		},
		// Different app — must not match.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "demo", Labels: map[string]string{"app": "other"}},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}

	pods, err := c.PodsForDeployment(context.Background(), "demo", "nginx")
	if err != nil {
		t.Fatalf("PodsForDeployment: %v", err)
	}
	if len(pods) != 2 {
		t.Fatalf("expected 2 pods, got %d (%v)", len(pods), pods)
	}
	want := map[string]bool{"nginx-a": true, "nginx-b": true}
	for _, p := range pods {
		if !want[p] {
			t.Errorf("unexpected pod %q in result", p)
		}
	}
}

func TestPodsForDeploymentMissingDeployment(t *testing.T) {
	t.Parallel()

	c := &Client{Clientset: fake.NewSimpleClientset(), Context: "test"}

	_, err := c.PodsForDeployment(context.Background(), "demo", "ghost")
	if err == nil {
		t.Error("expected error for missing deployment, got nil")
	}
}
