package k8s_test

import (
	"context"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func service(ns, name string, selector map[string]string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

func TestServiceLogTarget(t *testing.T) {
	t.Parallel()

	web := service("shop", "web", map[string]string{"app": "web"})
	sidecars := ownedPod("shop", "web-b", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodRunning)
	sidecars.Spec.Containers = []corev1.Container{{Name: "web"}, {Name: "proxy"}}
	plain := ownedPod("shop", "web-c", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodUnknown)
	failed := ownedPod("shop", "web-a", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodFailed)
	failed.Status.ContainerStatuses = []corev1.ContainerStatus{{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Error"}}}}
	done := ownedPod("shop", "web-job", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodSucceeded)
	stranger := ownedPod("shop", "db", map[string]string{"app": "db"}, metav1.OwnerReference{}, corev1.PodRunning)

	tests := []struct {
		name           string
		objs           []runtime.Object
		wantPods       []string
		wantContainers []string
		wantSkipped    map[string]k8s.SkipReason
		wantStatus     map[string]string
		wantMatched    int
		wantNote       string
	}{
		{
			name:           "containers come from the first streamed pod by name",
			objs:           []runtime.Object{web, plain, sidecars, stranger},
			wantPods:       []string{"web-b", "web-c"},
			wantContainers: []string{"web", "proxy"},
			wantMatched:    2,
		},
		{
			name:           "finished pods are skipped with their status",
			objs:           []runtime.Object{web, plain, failed, done},
			wantPods:       []string{"web-c"},
			wantContainers: []string{"app"},
			wantSkipped:    map[string]k8s.SkipReason{"web-a": k8s.SkipFinished, "web-job": k8s.SkipFinished},
			wantStatus:     map[string]string{"web-a": "Error", "web-job": string(corev1.PodSucceeded)},
			wantMatched:    3,
			wantNote:       "1 completed and 1 failed pods skipped (kubectl logs -l would include them)",
		},
		{
			name:        "only finished pods",
			objs:        []runtime.Object{web, done},
			wantSkipped: map[string]k8s.SkipReason{"web-job": k8s.SkipFinished},
			wantMatched: 1,
			wantNote:    "1 completed pod skipped (kubectl logs -l would include it)",
		},
		{
			name: "no pods at all",
			objs: []runtime.Object{web, stranger},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(tt.objs...)
			target, err := c.ServiceLogTarget(context.Background(), "shop", "web")
			if err != nil {
				t.Fatalf("ServiceLogTarget: %v", err)
			}
			checkTarget(t, target, tt.wantPods, nil, tt.wantSkipped, tt.wantMatched, tt.wantMatched > len(tt.wantPods))
			if !slices.Equal(target.Containers, tt.wantContainers) {
				t.Errorf("Containers = %q, want %q", target.Containers, tt.wantContainers)
			}
			for _, s := range target.Skipped {
				if want, ok := tt.wantStatus[s.Name]; ok && s.Status != want {
					t.Errorf("%s status = %q, want %q", s.Name, s.Status, want)
				}
			}
			if got := target.SkipNote(); got != tt.wantNote {
				t.Errorf("SkipNote() = %q, want %q", got, tt.wantNote)
			}
			if target.Kind != k8s.LogKindService || target.Selector != "app=web" {
				t.Errorf("target = %+v", target)
			}
		})
	}
}

func TestServiceLogTargetErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		objs    []runtime.Object
		forbid  string
		wantErr func(error) bool
	}{
		{name: "missing service", wantErr: apierrors.IsNotFound},
		{name: "selector-less service", objs: []runtime.Object{service("shop", "web", nil)}, wantErr: isNoSelector},
		{
			name: "pods forbidden", objs: []runtime.Object{service("shop", "web", map[string]string{"app": "web"})},
			forbid: "pods", wantErr: apierrors.IsForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(tt.objs...)
			if tt.forbid != "" {
				k8stest.Forbid(cs, "list", tt.forbid)
			}
			if _, err := c.ServiceLogTarget(context.Background(), "shop", "web"); !tt.wantErr(err) {
				t.Fatalf("err = %v, not the expected kind", err)
			}
		})
	}
}

func TestSkipNoteListsEveryKind(t *testing.T) {
	t.Parallel()

	target := k8s.LogTarget{Skipped: []k8s.SkippedPod{
		{Name: "a", Phase: string(corev1.PodSucceeded), Reason: k8s.SkipFinished, InSelector: true},
		{Name: "b", Phase: string(corev1.PodSucceeded), Reason: k8s.SkipFinished, InSelector: true},
		{Name: "c", Phase: string(corev1.PodFailed), Status: "Error", Reason: k8s.SkipFinished, InSelector: true},
		{Name: "d", Phase: string(corev1.PodFailed), Status: "Evicted", Reason: k8s.SkipFinished, InSelector: true},
		{Name: "e", Phase: string(corev1.PodRunning), Reason: k8s.SkipOtherOwner, InSelector: true},
		{Name: "f", Phase: string(corev1.PodSucceeded), Reason: k8s.SkipFinished, InSelector: false},
	}}
	want := "2 completed, 1 failed, 1 evicted and 1 other pods skipped (kubectl logs -l would include them)"
	if got := target.SkipNote(); got != want {
		t.Errorf("SkipNote() = %q\nwant         %q", got, want)
	}
}
