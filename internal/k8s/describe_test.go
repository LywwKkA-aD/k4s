package k8s

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestDescribePodAnnotations(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "noisy",
			Namespace: "demo",
			Annotations: map[string]string{
				lastAppliedConfigKey:        `{"apiVersion":"v1","kind":"Pod","metadata":{"name":"noisy"}}`,
				"useful-annotation":         "important",
				"with.newline/in/the/value": "line1\nline2\n",
				"definitely-too-long":       strings.Repeat("x", maxAnnotationLen+50),
			},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
	}

	out, err := fakeClient(pod).DescribePod(context.Background(), "demo", "noisy")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}
	if strings.Contains(out, lastAppliedConfigKey) {
		t.Error("last-applied-configuration is shown")
	}
	wants := []string{
		"useful-annotation: important\n",
		"with.newline/in/the/value:\n",
		"  line1\n",
		"  line2\n",
		"definitely-too-long:\n",
		"  " + strings.Repeat("x", maxAnnotationLen-2) + "...\n",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("annotations lack %q:\n%s", w, out)
		}
	}
}

func TestDescribePodWithoutStatus(t *testing.T) {
	t.Parallel()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "fresh", Namespace: "demo"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
	}
	out, err := fakeClient(pod).DescribePod(context.Background(), "demo", "fresh")
	if err != nil {
		t.Fatalf("DescribePod: %v", err)
	}
	for _, w := range []string{"Node:", "<none>", "IPs:", "Labels:", "Volumes:", "Events:"} {
		if !strings.Contains(out, w) {
			t.Errorf("an unscheduled pod's describe lacks %q:\n%s", w, out)
		}
	}
	if strings.Contains(out, "Container ID:") || strings.Contains(out, "Controlled By:") {
		t.Errorf("a pod without status or owner shows them:\n%s", out)
	}
}

func TestDescribeTerminatingPod(t *testing.T) {
	t.Parallel()

	pod := crashLoopPod()
	pod.DeletionTimestamp = ptr(when(describeNow.Add(-2 * time.Minute)))
	pod.DeletionGracePeriodSeconds = ptr[int64](30)
	pod.Status.Reason, pod.Status.Message = "Evicted", "low on memory"
	out, err := fakeClient(pod).describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
	if err != nil {
		t.Fatalf("describePod: %v", err)
	}
	for _, w := range []string{"Terminating (lasts 2m)", "Termination Grace Period:  30s", "Reason:", "Evicted", "low on memory"} {
		if !strings.Contains(out, w) {
			t.Errorf("a terminating pod lacks %q:\n%s", w, out)
		}
	}
}

func TestDescribeEventsOldestFirst(t *testing.T) {
	t.Parallel()

	pod := crashLoopPod()
	newer := podEvent(pod, "newer", corev1.EventTypeWarning, "BackOff", "newer", 1, before(time.Minute), before(time.Minute))
	older := podEvent(pod, "older", corev1.EventTypeNormal, "Pulled", "older", 1, before(time.Hour), before(time.Hour))
	out, err := fakeClient(pod, newer, older).describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
	if err != nil {
		t.Fatalf("describePod: %v", err)
	}
	o, n := strings.Index(out, "kubelet  older"), strings.Index(out, "kubelet  newer")
	if o < 0 || n < 0 || o > n {
		t.Errorf("events are not oldest first (older at %d, newer at %d):\n%s", o, n, out)
	}
	if !strings.Contains(out, "Warning  BackOff  60s   kubelet  newer") {
		t.Errorf("event row:\n%s", out)
	}
}

func TestShortenIsRuneSafe(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in    string
		limit int
		want  string
	}{
		{in: "short", limit: 10, want: "short"},
		{in: "abcdefghijkl", limit: 10, want: "abcdefghij..."},
		{in: "приветмир", limit: 5, want: "пр..."},
		{in: "🚀🚀🚀", limit: 6, want: "🚀..."},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			got := shorten(tc.in, tc.limit)
			if got != tc.want || !utf8.ValidString(got) {
				t.Errorf("shorten(%q, %d) = %q, want %q", tc.in, tc.limit, got, tc.want)
			}
		})
	}
}

func TestTolerationText(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   corev1.Toleration
		want string
	}{
		{name: "everything", in: corev1.Toleration{Operator: corev1.TolerationOpExists}, want: "op=Exists"},
		{name: "key and value", in: corev1.Toleration{Key: "gpu", Value: "a100", Effect: corev1.TaintEffectNoSchedule}, want: "gpu=a100:NoSchedule"},
		{name: "exists for a while", in: corev1.Toleration{Key: "k", Operator: corev1.TolerationOpExists, TolerationSeconds: ptr[int64](60)}, want: "k op=Exists for 60s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := toleration(tc.in); got != tc.want {
				t.Errorf("toleration = %q, want %q", got, tc.want)
			}
		})
	}
}
