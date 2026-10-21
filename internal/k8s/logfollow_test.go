package k8s_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

var podsGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// resolved is the deployment api in shop with pods h1-a and h1-b of its
// ReplicaSet h1, resolved the way the logs view gets it.
func resolved(t *testing.T, extra ...runtime.Object) (*k8s.Client, *fake.Clientset, k8s.LogTarget) {
	t.Helper()
	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	objs := append([]runtime.Object{api, replicaSet(api, "h1", "uid-h1", "1", 2),
		rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
		rsPod("shop", "api-h1-b", "api", "h1", "uid-h1", corev1.PodRunning),
	}, extra...)
	c, cs := k8stest.NewFakeClient(objs...)
	target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
	if err != nil {
		t.Fatalf("DeploymentLogTarget: %v", err)
	}
	cs.ClearActions()
	return c, cs, target
}

func reresolve(t *testing.T, c *k8s.Client, target k8s.LogTarget) k8s.LogTarget {
	t.Helper()
	next, err := c.ReresolveLogTarget(context.Background(), target)
	if err != nil {
		t.Fatalf("ReresolveLogTarget: %v", err)
	}
	return next
}

func TestDeploymentLogTargetKnowsItsReplicaSets(t *testing.T) {
	t.Parallel()

	_, _, target := resolved(t)
	if target.UID != "uid-api" {
		t.Errorf("UID = %q", target.UID)
	}
	want := []k8s.ReplicaSetRef{{UID: "uid-h1", Revision: 1, Owned: true}}
	if !slices.Equal(target.ReplicaSets, want) {
		t.Errorf("ReplicaSets = %+v, want %+v", target.ReplicaSets, want)
	}
	pod := target.Pods[0]
	if pod.Owner != "ReplicaSet/api-h1" || !slices.Equal(pod.Containers, []string{"app"}) || pod.Created.IsZero() {
		t.Errorf("pod ref = %+v", pod)
	}
	if c, ok := pod.State.Container(""); !ok || c.Name != "app" {
		t.Errorf("pod state = %+v", pod.State)
	}
}

func TestReresolveFollowsARollout(t *testing.T) {
	t.Parallel()

	c, cs, target := resolved(t)
	steady := reresolve(t, c, target)
	if !slices.Equal(steady.PodNames(), target.PodNames()) || !slices.Equal(steady.Hashes, []string{"h1"}) {
		t.Fatalf("steady re-resolve = %v %v", steady.PodNames(), steady.Hashes)
	}
	checkCalls(t, cs.Actions(), []apiCall{{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"}})

	// The rollout: a new ReplicaSet with a pod, an old pod on its way out.
	cs.ClearActions()
	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	for _, obj := range []runtime.Object{replicaSet(api, "h2", "uid-h2", "2", 1), rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodPending)} {
		if err := cs.Tracker().Add(obj); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if err := cs.Tracker().Delete(podsGVR, "shop", "api-h1-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rolled := reresolve(t, c, steady)
	if want := []string{"api-h1-b", "api-h2-a"}; !slices.Equal(rolled.PodNames(), want) {
		t.Errorf("pods after the rollout = %v, want %v", rolled.PodNames(), want)
	}
	if want := []string{"h2", "h1"}; !slices.Equal(rolled.Hashes, want) {
		t.Errorf("Hashes = %v, want %v", rolled.Hashes, want)
	}
	checkCalls(t, cs.Actions(), []apiCall{
		{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"},
		{Verb: "list", Resource: "replicasets", Selector: "app=api"},
	})

	// The new ReplicaSet is known now: the next round lists pods only.
	cs.ClearActions()
	reresolve(t, c, rolled)
	checkCalls(t, cs.Actions(), []apiCall{{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"}})
}

func TestReresolveLeavesOtherOwnersOut(t *testing.T) {
	t.Parallel()

	worker := deployment("shop", "api-worker", "uid-worker", map[string]string{"app": "api"})
	workerPod := rsPod("shop", "api-worker-w1-a", "api-worker", "w1", "uid-w1", corev1.PodRunning)
	workerPod.Labels["app"] = "api"
	job := ownedPod("shop", "api-cron-1", map[string]string{"app": "api"}, controllerRef("batch/v1", "Job", "api-cron", "uid-job"), corev1.PodRunning)
	c, cs, target := resolved(t, worker)

	// The other deployment rolls out after the logs opened.
	for _, obj := range []runtime.Object{replicaSet(worker, "w1", "uid-w1", "1", 1), workerPod, job} {
		if err := cs.Tracker().Add(obj); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	first := reresolve(t, c, target)
	second := reresolve(t, c, first)
	for _, got := range []k8s.LogTarget{first, second} {
		if want := []string{"api-h1-a", "api-h1-b"}; !slices.Equal(got.PodNames(), want) {
			t.Errorf("pods = %v, want %v", got.PodNames(), want)
		}
	}
	// The other deployment's ReplicaSet is named like api's; one quorum
	// list tells it apart, and it is remembered. The CronJob's pod never
	// asks for one.
	checkCalls(t, cs.Actions(), []apiCall{
		{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"},
		{Verb: "list", Resource: "replicasets", Selector: "app=api"},
		{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"},
	})
	if len(second.Skipped) != 2 {
		t.Errorf("skipped = %+v", second.Skipped)
	}
}

func TestReresolveGuessedOwnersNeverListReplicaSets(t *testing.T) {
	t.Parallel()

	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	c, cs := k8stest.NewFakeClient(api, rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning))
	k8stest.Forbid(cs, "list", "replicasets")
	target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
	if err != nil {
		t.Fatalf("DeploymentLogTarget: %v", err)
	}
	if err := cs.Tracker().Add(rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodRunning)); err != nil {
		t.Fatalf("add: %v", err)
	}
	cs.ClearActions()
	next := reresolve(t, c, target)
	if !next.OwnersGuessed || !slices.Equal(next.PodNames(), []string{"api-h1-a", "api-h2-a"}) {
		t.Errorf("guessed re-resolve = %+v", next)
	}
	checkCalls(t, cs.Actions(), []apiCall{{Verb: "list", Resource: "pods", Selector: "app=api", ResourceVersion: "0"}})
}

func TestReresolveAHandBuiltTargetResolvesAfresh(t *testing.T) {
	t.Parallel()

	c, cs, _ := resolved(t)
	bare := k8s.LogTarget{Kind: k8s.LogKindDeployment, Namespace: "shop", Name: "api", Selector: "app=api"}
	next := reresolve(t, c, bare)
	if next.UID != "uid-api" || !slices.Equal(next.PodNames(), []string{"api-h1-a", "api-h1-b"}) {
		t.Errorf("re-resolve of a bare target = %+v", next)
	}
	if calls := apiCalls(cs.Actions()); len(calls) == 0 || calls[0].Resource != "pods" {
		t.Errorf("calls = %+v", calls)
	}
}

func TestReresolveService(t *testing.T) {
	t.Parallel()

	web := service("mall", "front", map[string]string{"app": "web"})
	a := ownedPod("mall", "web-a", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodRunning)
	c, cs := k8stest.NewFakeClient(web, a)
	target, err := c.ServiceLogTarget(context.Background(), "mall", "front")
	if err != nil {
		t.Fatalf("ServiceLogTarget: %v", err)
	}
	b := ownedPod("mall", "web-b", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodRunning)
	b.Spec.Containers = []corev1.Container{{Name: "other"}}
	b.CreationTimestamp = metav1.NewTime(time.Unix(1_700_000_900, 0))
	done := ownedPod("mall", "web-job", map[string]string{"app": "web"}, metav1.OwnerReference{}, corev1.PodSucceeded)
	for _, obj := range []runtime.Object{b, done} {
		if err := cs.Tracker().Add(obj); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	cs.ClearActions()
	next := reresolve(t, c, target)
	if want := []string{"web-a", "web-b"}; !slices.Equal(next.PodNames(), want) {
		t.Errorf("pods = %v, want %v", next.PodNames(), want)
	}
	if !slices.Equal(next.Containers, target.Containers) {
		t.Errorf("Containers = %v, want them kept: %v", next.Containers, target.Containers)
	}
	if got := next.Pods[1]; !slices.Equal(got.Containers, []string{"other"}) || !got.Created.Equal(b.CreationTimestamp.Time) {
		t.Errorf("joined pod = %+v", got)
	}
	if len(next.Skipped) != 1 || next.Skipped[0].State.Phase != "Succeeded" {
		t.Errorf("skipped = %+v", next.Skipped)
	}
	checkCalls(t, cs.Actions(), []apiCall{{Verb: "list", Resource: "pods", Selector: "app=web", ResourceVersion: "0"}})
}

func TestReresolveErrors(t *testing.T) {
	t.Parallel()

	c, cs, target := resolved(t)
	cs.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewServiceUnavailable("down")
	})
	if _, err := c.ReresolveLogTarget(context.Background(), target); !apierrors.IsServiceUnavailable(err) {
		t.Errorf("pods list failure: err = %v", err)
	}
	rc, rcs, rolling := resolved(t, rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodRunning))
	rcs.PrependReactor("list", "replicasets", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("Unauthorized")
	})
	if _, err := rc.ReresolveLogTarget(context.Background(), rolling); !apierrors.IsUnauthorized(err) {
		t.Errorf("replicaset re-list failure: err = %v", err)
	}
	if _, err := c.ReresolveLogTarget(context.Background(), k8s.LogTarget{Kind: "statefulset"}); !errors.Is(err, k8s.ErrUnsupportedKind) {
		t.Errorf("unsupported kind: err = %v", err)
	}
	if _, err := c.ReresolveLogTarget(context.Background(), k8s.LogTarget{Kind: k8s.LogKindService}); !errors.Is(err, k8s.ErrNoSelector) {
		t.Errorf("no selector: err = %v", err)
	}
}
