package k8s_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// footer builds the logs command the way the views will, from a target.
func footer(t *testing.T, target k8s.LogTarget, tail int64) kubectl.Cmd {
	t.Helper()
	cmd, err := kubectl.SelectorLogs(kubectl.LogsSpec{
		Namespace: target.Namespace,
		Selector:  kubectl.HashSelector(target.Selector, target.Hashes),
		Tail:      tail,
		Matched:   target.Matched(),
		Streamed:  len(target.Pods),
	})
	if err != nil {
		t.Fatalf("SelectorLogs: %v", err)
	}
	return cmd
}

// matching counts the objects in objs that are pods matched by selector, as
// `kubectl get pods -l` would.
func matching(t *testing.T, objs []runtime.Object, selector string) int {
	t.Helper()
	sel, err := labels.Parse(selector)
	if err != nil {
		t.Fatalf("parse selector %q: %v", selector, err)
	}
	n := 0
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok && sel.Matches(labels.Set(p.Labels)) {
			n++
		}
	}
	return n
}

func sortedRunning() []string {
	return slices.Sorted(slices.Values(glRunning))
}

func TestDeploymentLogTargetGrowlytics(t *testing.T) {
	t.Parallel()

	objs := growlyticsObjects()
	c, cs := k8stest.NewFakeClient(objs...)
	target, err := c.DeploymentLogTarget(context.Background(), glNS, glApp)
	if err != nil {
		t.Fatalf("DeploymentLogTarget: %v", err)
	}

	if got := target.PodNames(); !slices.Equal(got, sortedRunning()) {
		t.Errorf("streamed pods = %q, want exactly the 2 running ReplicaSet pods %q", got, sortedRunning())
	}
	if !slices.Equal(target.Hashes, []string{glHash}) {
		t.Errorf("Hashes = %q, want [%s]", target.Hashes, glHash)
	}
	if target.Selector != "app="+glApp {
		t.Errorf("Selector = %q, want the bare deployment selector", target.Selector)
	}
	if !slices.Equal(target.Containers, []string{"app"}) {
		t.Errorf("Containers = %q, want [app]", target.Containers)
	}
	checkGrowlyticsSkips(t, target.Skipped)

	shown := kubectl.HashSelector(target.Selector, target.Hashes)
	if got, want := target.Matched(), matching(t, objs, shown); got != want || got != 3 {
		t.Errorf("Matched() = %d, kubectl get pods -l %q matches %d; want both 3", got, shown, want)
	}
	if !target.Approx() {
		t.Error("Approx() = false with an evicted pod inside the shown selector")
	}
	if got, want := target.SkipNote(), "1 evicted pod skipped (kubectl logs -l would include it)"; got != want {
		t.Errorf("SkipNote() = %q, want %q", got, want)
	}

	cmd := footer(t, target, 100)
	want := "kubectl logs -f -l 'app=growlytics-app,pod-template-hash in (7d9f8c6b5d)' --prefix --tail=100 --max-log-requests=3 -n growlytics"
	if cmd.String() != want {
		t.Errorf("footer = %s\nwant     %s", cmd.String(), want)
	}

	checkCalls(t, cs.Actions(), []apiCall{
		{Verb: "get", Resource: "deployments"},
		{Verb: "list", Resource: "replicasets", Selector: "app=" + glApp, ResourceVersion: "0"},
		{Verb: "list", Resource: "pods", Selector: "app=" + glApp, ResourceVersion: "0"},
	})
}

// checkGrowlyticsSkips expects the evicted pod skipped as finished inside
// the shown selector and the 13 Job pods skipped as other owners outside it.
func checkGrowlyticsSkips(t *testing.T, skipped []k8s.SkippedPod) {
	t.Helper()
	var jobs int
	for _, s := range skipped {
		switch {
		case s.Name == glEvicted:
			if s.Reason != k8s.SkipFinished || s.Status != "Evicted" || !s.InSelector {
				t.Errorf("evicted pod skip = %+v, want finished, Evicted, inside the shown selector", s)
			}
		case s.Reason == k8s.SkipOtherOwner && !s.InSelector && strings.HasPrefix(s.Owner, "Job/growlytics-cron") && s.Phase == string(corev1.PodSucceeded):
			jobs++
		default:
			t.Errorf("unexpected skip %+v", s)
		}
	}
	if jobs != 13 || len(skipped) != 14 {
		t.Errorf("skipped %d pods with %d Job pods, want the evicted pod and 13 Job pods", len(skipped), jobs)
	}
}

func TestServiceLogTargetGrowlytics(t *testing.T) {
	t.Parallel()

	objs := growlyticsObjects()
	c, cs := k8stest.NewFakeClient(objs...)
	target, err := c.ServiceLogTarget(context.Background(), glNS, glApp)
	if err != nil {
		t.Fatalf("ServiceLogTarget: %v", err)
	}

	if got := target.PodNames(); !slices.Equal(got, sortedRunning()) {
		t.Errorf("streamed pods = %q, want %q", got, sortedRunning())
	}
	if len(target.Skipped) != 14 {
		t.Fatalf("skipped %d pods, want the 13 Job pods and the evicted one", len(target.Skipped))
	}
	for _, s := range target.Skipped {
		if s.Reason != k8s.SkipFinished || !s.InSelector {
			t.Errorf("skip = %+v, want a finished pod inside the selector", s)
		}
	}
	if target.Hashes != nil {
		t.Errorf("Hashes = %q, want none for a service", target.Hashes)
	}
	if got, want := target.Matched(), matching(t, objs, target.Selector); got != want || got != 16 {
		t.Errorf("Matched() = %d, kubectl get pods -l matches %d; want both 16", got, want)
	}
	if got, want := target.SkipNote(), "13 completed and 1 evicted pods skipped (kubectl logs -l would include them)"; got != want {
		t.Errorf("SkipNote() = %q, want %q", got, want)
	}

	cmd := footer(t, target, 100)
	want := "kubectl logs -f -l app=growlytics-app --prefix --tail=100 --max-log-requests=16 -n growlytics"
	if cmd.String() != want {
		t.Errorf("String() = %s\nwant       %s", cmd.String(), want)
	}
	if cmd.Display() != "≈ "+want {
		t.Errorf("Display() = %s, want the approximate mark", cmd.Display())
	}

	checkCalls(t, cs.Actions(), []apiCall{
		{Verb: "get", Resource: "services"},
		{Verb: "list", Resource: "pods", Selector: "app=" + glApp, ResourceVersion: "0"},
	})
}

func TestServiceLogTargetFifteenPodsRaiseTheRequestCap(t *testing.T) {
	t.Parallel()

	objs := slices.DeleteFunc(growlyticsObjects(), func(o runtime.Object) bool {
		p, ok := o.(*corev1.Pod)
		return ok && p.Name == glEvicted
	})
	c, _ := k8stest.NewFakeClient(objs...)
	target, err := c.ServiceLogTarget(context.Background(), glNS, glApp)
	if err != nil {
		t.Fatalf("ServiceLogTarget: %v", err)
	}
	if got := footer(t, target, 10).String(); !strings.Contains(got, " --max-log-requests=15 ") {
		t.Errorf("footer = %s, want --max-log-requests=15", got)
	}
	if got, want := target.SkipNote(), "13 completed pods skipped (kubectl logs -l would include them)"; got != want {
		t.Errorf("SkipNote() = %q, want %q", got, want)
	}
}

func TestResolveLogTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind     string
		wantKind string
		wantErr  error
	}{
		{kind: k8s.LogKindDeployment, wantKind: k8s.LogKindDeployment},
		{kind: k8s.LogKindService, wantKind: k8s.LogKindService},
		{kind: "pod", wantErr: k8s.ErrUnsupportedKind},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(growlyticsObjects()...)
			target, err := c.ResolveLogTarget(context.Background(), tt.kind, glNS, glApp)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if target.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", target.Kind, tt.wantKind)
			}
			if tt.wantErr == nil && (target.Namespace != glNS || target.Name != glApp || len(target.Pods) != 2) {
				t.Errorf("target = %+v, want 2 pods of %s/%s", target, glNS, glApp)
			}
		})
	}
}
