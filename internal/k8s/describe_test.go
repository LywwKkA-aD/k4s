package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDescribePodRendersExpectedSections(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nginx-7d",
			Namespace:         "demo",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-5 * time.Minute)),
			Labels:            map[string]string{"app": "nginx"},
			Annotations:       map[string]string{"k8s.io/managed-by": "k4s-test"},
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.27-alpine",
					Ports: []corev1.ContainerPort{{ContainerPort: 80, Protocol: corev1.ProtocolTCP}},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10m"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("100m"),
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: "greeting", MountPath: "/usr/share/nginx/html", ReadOnly: true},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "greeting",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "nginx-greeting"},
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase:    corev1.PodRunning,
			PodIP:    "10.0.0.5",
			QOSClass: corev1.PodQOSBurstable,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "nginx",
					Ready:        true,
					RestartCount: 0,
					State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: metav1.Now()}},
				},
			},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(pod)), Context: "test"}

	out, err := c.DescribePod(context.Background(), "demo", "nginx-7d")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}

	mustContain := []string{
		"Name:", "nginx-7d",
		"Namespace:", "demo",
		"Node:", "node-1",
		"Status:", "Running",
		"Pod IP:", "10.0.0.5",
		"QoS Class:", "Burstable",
		"Labels:", "app=nginx",
		"Annotations:", "k8s.io/managed-by=k4s-test",
		"Containers:",
		"Image:", "nginx:1.27-alpine",
		"Ports:", "80/TCP",
		"Resources:",
		"requests.cpu:",
		"limits.cpu:",
		"Mounts:",
		"/usr/share/nginx/html from greeting (ro)",
		"Conditions:",
		"Ready",
		"Volumes:",
		"type:  configMap",
	}

	for _, frag := range mustContain {
		if !strings.Contains(out, frag) {
			t.Errorf("describe output missing %q\n---\n%s\n---", frag, out)
		}
	}
}

func TestDescribePodWaitingContainerSurfacesReason(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "demo", CreationTimestamp: metav1.Now()},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "busybox"}}},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "main",
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				},
			},
		},
	}

	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(pod)), Context: "test"}

	out, err := c.DescribePod(context.Background(), "demo", "broken")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}
	if !strings.Contains(out, "CrashLoopBackOff") {
		t.Errorf("waiting reason missing from describe output:\n%s", out)
	}
	if !strings.Contains(out, "Restart Count:  5") {
		t.Errorf("restart count missing from describe output:\n%s", out)
	}
}
