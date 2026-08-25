package k8s

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	appsv1 "k8s.io/api/apps/v1"
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

func TestDescribePodSkipsLastAppliedConfigurationAnnotation(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noisy",
			Namespace: "demo",
			Annotations: map[string]string{
				lastAppliedConfigKey:        `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"noisy"}}`,
				"useful-annotation":         "important",
				"with.newline/in/the/value": "line1\nline2\nline3",
				"definitely-too-long":       strings.Repeat("x", maxAnnotationValueLen+50),
			},
		},
		Spec:   corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(pod)), Context: "test"}

	out, err := c.DescribePod(context.Background(), "demo", "noisy")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}

	if strings.Contains(out, lastAppliedConfigKey) {
		t.Error("output should not contain the last-applied-configuration annotation")
	}
	if !strings.Contains(out, "useful-annotation=important") {
		t.Error("non-noise annotation must be visible")
	}
	if strings.Contains(out, "line1\nline2") {
		t.Error("annotation values must not carry literal newlines into the output")
	}
	if !strings.Contains(out, "with.newline/in/the/value=line1 line2 line3") {
		t.Errorf("multiline annotation should be flattened to spaces, got:\n%s", out)
	}
	if !strings.Contains(out, "…") {
		t.Errorf("very long annotation should be truncated with an ellipsis, got:\n%s", out)
	}
}

func TestDescribePodIncludesEvents(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "watched", Namespace: "demo"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	now := metav1.Now()
	earlier := metav1.NewTime(now.Add(-5 * time.Minute))

	failedEvent := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "watched.fail", Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "watched", Namespace: "demo"},
		Type:           "Warning",
		Reason:         "BackOff",
		Message:        "Back-off restarting failed container",
		LastTimestamp:  now,
	}
	pulledEvent := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "watched.pull", Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "watched", Namespace: "demo"},
		Type:           "Normal",
		Reason:         "Pulled",
		Message:        "Successfully pulled image",
		LastTimestamp:  earlier,
	}
	otherPodEvent := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "other.event", Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "someone-else", Namespace: "demo"},
		Type:           "Warning",
		Reason:         "ShouldNotAppear",
		Message:        "this event belongs to a different pod",
		LastTimestamp:  now,
	}

	c := &Client{
		Clientset: fake.NewSimpleClientset(
			runtime.Object(pod),
			runtime.Object(failedEvent),
			runtime.Object(pulledEvent),
			runtime.Object(otherPodEvent),
		),
		Context: "test",
	}

	out, err := c.DescribePod(context.Background(), "demo", "watched")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}

	if !strings.Contains(out, "Events:") {
		t.Errorf("output missing Events section:\n%s", out)
	}
	if !strings.Contains(out, "Back-off restarting failed container") {
		t.Errorf("recent event message missing:\n%s", out)
	}
	if !strings.Contains(out, "Successfully pulled image") {
		t.Errorf("older event message missing:\n%s", out)
	}
	if strings.Contains(out, "ShouldNotAppear") {
		t.Error("event for a different pod must be filtered out")
	}

	// Most-recent-first ordering: BackOff (now) must precede Pulled (earlier).
	backOffIdx := strings.Index(out, "BackOff")
	pulledIdx := strings.Index(out, "Pulled")
	if backOffIdx == -1 || pulledIdx == -1 || backOffIdx >= pulledIdx {
		t.Errorf("events should be sorted newest first; backoff=%d pulled=%d:\n%s", backOffIdx, pulledIdx, out)
	}
}

func TestDescribeDeploymentRendersHeaderReplicasTemplateAndEvents(t *testing.T) {
	t.Parallel()

	desired := int32(3)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nginx",
			Namespace:         "demo",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-10 * time.Minute)),
			Labels:            map[string]string{"app": "nginx"},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &desired,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "nginx"}},
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RollingUpdateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx", "tier": "front"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "nginx", Image: "nginx:1.27-alpine"},
					},
				},
			},
		},
		Status: appsv1.DeploymentStatus{
			Replicas:          3,
			ReadyReplicas:     2,
			UpdatedReplicas:   3,
			AvailableReplicas: 2,
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue, Reason: "MinimumReplicasAvailable"},
			},
		},
	}
	scaledEvent := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "nginx.scaled", Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{Kind: "Deployment", Name: "nginx", Namespace: "demo"},
		Type:           "Normal",
		Reason:         "ScalingReplicaSet",
		Message:        "Scaled up replica set nginx-7d to 3",
		LastTimestamp:  metav1.Now(),
	}

	c := &Client{
		Clientset: fake.NewSimpleClientset(runtime.Object(dep), runtime.Object(scaledEvent)),
		Context:   "test",
	}

	out, err := c.DescribeDeployment(context.Background(), "demo", "nginx")
	if err != nil {
		t.Fatalf("DescribeDeployment: %v", err)
	}

	mustContain := []string{
		"Name:", "nginx",
		"Namespace:", "demo",
		"Selector:", "app=nginx",
		"Strategy:", "RollingUpdate",
		"Replicas:", "3 desired", "2 ready", "3 updated", "2 available",
		"Pod Template:",
		"Labels: app=nginx, tier=front",
		"Containers:", "nginx:1.27-alpine",
		"Conditions:", "MinimumReplicasAvailable",
		"Events:", "ScalingReplicaSet", "Scaled up replica set",
	}
	for _, frag := range mustContain {
		if !strings.Contains(out, frag) {
			t.Errorf("describe output missing %q\n---\n%s\n---", frag, out)
		}
	}
}

func TestDescribeServiceRendersTypeIPsPortsSelectorEvents(t *testing.T) {
	t.Parallel()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "nginx",
			Namespace:         "demo",
			CreationTimestamp: metav1.NewTime(time.Now().Add(-time.Hour)),
			Labels:            map[string]string{"app": "nginx"},
		},
		Spec: corev1.ServiceSpec{
			Type:            corev1.ServiceTypeNodePort,
			ClusterIP:       "10.0.0.5",
			Selector:        map[string]string{"app": "nginx"},
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports: []corev1.ServicePort{
				{Name: "http", Port: 80, NodePort: 31000, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "nginx.up", Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{Kind: "Service", Name: "nginx", Namespace: "demo"},
		Type:           "Normal",
		Reason:         "EnsuringLoadBalancer",
		Message:        "Ensuring load balancer",
		LastTimestamp:  metav1.Now(),
	}

	c := &Client{Clientset: fake.NewSimpleClientset(runtime.Object(svc), runtime.Object(event)), Context: "test"}

	out, err := c.DescribeService(context.Background(), "demo", "nginx")
	if err != nil {
		t.Fatalf("DescribeService: %v", err)
	}

	mustContain := []string{
		"Name:", "nginx",
		"Namespace:", "demo",
		"Type:", "NodePort",
		"Cluster IP:", "10.0.0.5",
		"Selector:", "app=nginx",
		"Session Affinity:", "None",
		"Ports:",
		"http", "80/TCP",
		"NodePort 31000",
		"Events:", "EnsuringLoadBalancer", "Ensuring load balancer",
	}
	for _, frag := range mustContain {
		if !strings.Contains(out, frag) {
			t.Errorf("service describe output missing %q\n---\n%s\n---", frag, out)
		}
	}
}

func TestTruncateStringMultibyteSafe(t *testing.T) {
	t.Parallel()
	// Multi-byte runes used to be sliced mid-sequence by the byte-based
	// truncation, producing invalid UTF-8 in the describe view.
	for _, s := range []string{"привет мир, это длинная строка", "🚀🚀🚀🚀🚀 emoji 🚀🚀🚀🚀🚀"} {
		got := truncateString(s, 10)
		if !utf8.ValidString(got) {
			t.Errorf("truncateString(%q, 10) produced invalid UTF-8: %q", s, got)
		}
		if n := utf8.RuneCountInString(got); n > 10 {
			t.Errorf("truncateString(%q, 10) = %q with %d runes, want <= 10", s, got, n)
		}
	}
}

func TestTruncateStringShortStringUnchanged(t *testing.T) {
	t.Parallel()
	if got := truncateString("short", 10); got != "short" {
		t.Errorf("truncateString = %q, want %q", got, "short")
	}
}
