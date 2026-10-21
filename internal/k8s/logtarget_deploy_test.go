package k8s_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func at(sec int64) metav1.Time { return metav1.NewTime(time.Unix(1_700_000_000+sec, 0)) }

func TestDeploymentLogTargetOwnerChain(t *testing.T) {
	t.Parallel()

	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	worker := deployment("shop", "api-worker", "uid-worker", map[string]string{"app": "api"})

	terminating := rsPod("shop", "api-h1-term", "api", "h1", "uid-h1", corev1.PodRunning)
	terminating.DeletionTimestamp = &metav1.Time{Time: time.Unix(1_700_000_100, 0)}
	noHash := rsPod("shop", "api-h1-nohash", "api", "h1", "uid-h1", corev1.PodRunning)
	delete(noHash.Labels, hashKey)
	oldNoRevision := replicaSet(api, "h0", "uid-h0", "", 1)
	oldNoRevision.CreationTimestamp = at(-100)
	// Same selector as api, so its ReplicaSet and pod list under api's too.
	workerPod := rsPod("shop", "api-worker-w1-a", "api-worker", "w1", "uid-w1", corev1.PodRunning)
	workerPod.Labels["app"] = "api"

	tests := []struct {
		name         string
		objs         []runtime.Object
		wantPods     []string
		wantHashes   []string
		wantSkipped  map[string]k8s.SkipReason
		wantOwner    map[string]string
		wantMatched  int
		wantApprox   bool
		wantInSelect map[string]bool
	}{
		{
			name: "rollout lists the new hash first",
			objs: []runtime.Object{api,
				replicaSet(api, "h1", "uid-h1", "3", 1), replicaSet(api, "h2", "uid-h2", "4", 1),
				rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
				rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodPending),
			},
			wantPods:    []string{"api-h1-a", "api-h2-a"},
			wantHashes:  []string{"h2", "h1"},
			wantMatched: 2,
		},
		{
			name: "equal revisions sort by hash",
			objs: []runtime.Object{api,
				replicaSet(api, "hb", "uid-hb", "2", 1), replicaSet(api, "ha", "uid-ha", "2", 1),
				rsPod("shop", "api-hb-a", "api", "hb", "uid-hb", corev1.PodRunning),
				rsPod("shop", "api-ha-a", "api", "ha", "uid-ha", corev1.PodRunning),
			},
			wantPods:    []string{"api-ha-a", "api-hb-a"},
			wantHashes:  []string{"ha", "hb"},
			wantMatched: 2,
		},
		{
			name: "a missing revision sorts after numbered ones",
			objs: []runtime.Object{api, oldNoRevision, replicaSet(api, "h1", "uid-h1", "1", 1),
				rsPod("shop", "api-h0-a", "api", "h0", "uid-h0", corev1.PodRunning),
				rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
			},
			wantPods:    []string{"api-h0-a", "api-h1-a"},
			wantHashes:  []string{"h1", "h0"},
			wantMatched: 2,
		},
		{
			name: "another deployment's ReplicaSet sharing the labels is not ours",
			objs: []runtime.Object{api, worker,
				replicaSet(api, "h1", "uid-h1", "1", 1), replicaSet(worker, "w1", "uid-w1", "1", 1),
				rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
				workerPod,
			},
			wantPods:     []string{"api-h1-a"},
			wantHashes:   []string{"h1"},
			wantSkipped:  map[string]k8s.SkipReason{"api-worker-w1-a": k8s.SkipOtherOwner},
			wantOwner:    map[string]string{"api-worker-w1-a": "ReplicaSet/api-worker-w1"},
			wantInSelect: map[string]bool{"api-worker-w1-a": false},
			wantMatched:  1,
		},
		{
			name: "a bare pod with the labels has no owner",
			objs: []runtime.Object{api, replicaSet(api, "h1", "uid-h1", "1", 1),
				rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
				ownedPod("shop", "debug", map[string]string{"app": "api", hashKey: "h1"}, metav1.OwnerReference{}, corev1.PodRunning),
			},
			wantPods:     []string{"api-h1-a"},
			wantHashes:   []string{"h1"},
			wantSkipped:  map[string]k8s.SkipReason{"debug": k8s.SkipOtherOwner},
			wantOwner:    map[string]string{"debug": ""},
			wantInSelect: map[string]bool{"debug": true},
			wantMatched:  2,
			wantApprox:   true,
		},
		{
			name:        "terminating pods still stream",
			objs:        []runtime.Object{api, replicaSet(api, "h1", "uid-h1", "1", 1), terminating},
			wantPods:    []string{"api-h1-term"},
			wantHashes:  []string{"h1"},
			wantMatched: 1,
		},
		{
			name: "a streamed pod without the hash label shows the bare selector",
			objs: []runtime.Object{api, replicaSet(api, "h1", "uid-h1", "1", 1), noHash,
				rsPod("shop", "api-h1-done", "api", "h1", "uid-h1", corev1.PodSucceeded),
			},
			wantPods:     []string{"api-h1-nohash"},
			wantSkipped:  map[string]k8s.SkipReason{"api-h1-done": k8s.SkipFinished},
			wantInSelect: map[string]bool{"api-h1-done": true},
			wantMatched:  2,
			wantApprox:   true,
		},
		{
			name: "no running pods leaves every match skipped",
			objs: []runtime.Object{api, replicaSet(api, "h1", "uid-h1", "1", 0),
				rsPod("shop", "api-h1-done", "api", "h1", "uid-h1", corev1.PodFailed),
			},
			wantSkipped:  map[string]k8s.SkipReason{"api-h1-done": k8s.SkipFinished},
			wantInSelect: map[string]bool{"api-h1-done": true},
			wantMatched:  1,
			wantApprox:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, _ := k8stest.NewFakeClient(tt.objs...)
			target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
			if err != nil {
				t.Fatalf("DeploymentLogTarget: %v", err)
			}
			checkTarget(t, target, tt.wantPods, tt.wantHashes, tt.wantSkipped, tt.wantMatched, tt.wantApprox)
			for _, s := range target.Skipped {
				if want, ok := tt.wantOwner[s.Name]; ok && s.Owner != want {
					t.Errorf("%s owner = %q, want %q", s.Name, s.Owner, want)
				}
				if want, ok := tt.wantInSelect[s.Name]; ok && s.InSelector != want {
					t.Errorf("%s InSelector = %v, want %v", s.Name, s.InSelector, want)
				}
			}
			if target.OwnersGuessed {
				t.Error("OwnersGuessed set although ReplicaSets were listed")
			}
		})
	}
}

func checkTarget(t *testing.T, target k8s.LogTarget, pods, hashes []string, skipped map[string]k8s.SkipReason, matched int, approx bool) {
	t.Helper()
	if got := target.PodNames(); !slices.Equal(got, pods) {
		t.Errorf("pods = %q, want %q", got, pods)
	}
	if !slices.Equal(target.Hashes, hashes) {
		t.Errorf("Hashes = %q, want %q", target.Hashes, hashes)
	}
	gotSkipped := map[string]k8s.SkipReason{}
	for _, s := range target.Skipped {
		gotSkipped[s.Name] = s.Reason
	}
	if len(gotSkipped) != len(skipped) {
		t.Errorf("skipped = %v, want %v", gotSkipped, skipped)
	}
	for name, reason := range skipped {
		if gotSkipped[name] != reason {
			t.Errorf("skip %s = %q, want %q", name, gotSkipped[name], reason)
		}
	}
	if got := target.Matched(); got != matched {
		t.Errorf("Matched() = %d, want %d", got, matched)
	}
	if got := target.Approx(); got != approx {
		t.Errorf("Approx() = %v, want %v", got, approx)
	}
}

func TestDeploymentLogTargetGuessesOwnersWhenReplicaSetsAreForbidden(t *testing.T) {
	t.Parallel()

	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	older := rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning)
	older.CreationTimestamp = at(0)
	newer := rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodRunning)
	newer.CreationTimestamp = at(60)
	alsoNewer := rsPod("shop", "api-h2-b", "api", "h2", "uid-h2", corev1.PodRunning)
	alsoNewer.CreationTimestamp = at(30)
	job := ownedPod("shop", "api-cron-1", map[string]string{"app": "api"}, controllerRef("batch/v1", "Job", "api-cron", "uid-job"), corev1.PodSucceeded)
	impostor := rsPod("shop", "api-zz-a", "other", "zz", "uid-zz", corev1.PodRunning)
	impostor.Labels["app"] = "api"

	c, cs := k8stest.NewFakeClient(api, older, newer, alsoNewer, job, impostor)
	k8stest.Forbid(cs, "list", "replicasets")

	target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
	if err != nil {
		t.Fatalf("DeploymentLogTarget: %v", err)
	}
	if !target.OwnersGuessed {
		t.Error("OwnersGuessed = false after a forbidden ReplicaSet list")
	}
	checkTarget(t, target,
		[]string{"api-h1-a", "api-h2-a", "api-h2-b"},
		[]string{"h2", "h1"},
		map[string]k8s.SkipReason{"api-cron-1": k8s.SkipOtherOwner, "api-zz-a": k8s.SkipOtherOwner},
		3, true)
}

// The ReplicaSet and pod lists come from separate watch caches, so a pod of
// a ReplicaSet created a moment ago can outrun the ReplicaSet list.
func TestDeploymentLogTargetRelistsStaleReplicaSets(t *testing.T) {
	t.Parallel()

	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	oldRS := replicaSet(api, "h1", "uid-h1", "3", 1)
	newRS := replicaSet(api, "h2", "uid-h2", "4", 1)
	// Controlled by another deployment's ReplicaSet that shares the labels.
	foreign := rsPod("shop", "other-x1-a", "other", "x1", "uid-foreign", corev1.PodRunning)
	foreign.Labels["app"] = "api"
	objs := []runtime.Object{api, oldRS, newRS,
		rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning),
		rsPod("shop", "api-h2-a", "api", "h2", "uid-h2", corev1.PodRunning),
		foreign,
	}

	tests := []struct {
		name      string
		objs      []runtime.Object
		stale     bool
		relistErr error
		want      []string
		wantHash  []string
		wantSkip  map[string]k8s.SkipReason
		wantLists []string
	}{
		{
			name: "a stale list is replaced by a fresh one", objs: objs, stale: true,
			want: []string{"api-h1-a", "api-h2-a"}, wantHash: []string{"h2", "h1"},
			wantSkip:  map[string]k8s.SkipReason{"other-x1-a": k8s.SkipOtherOwner},
			wantLists: []string{"0", ""},
		},
		{
			name: "a current list is not read twice", objs: objs,
			want: []string{"api-h1-a", "api-h2-a"}, wantHash: []string{"h2", "h1"},
			wantSkip:  map[string]k8s.SkipReason{"other-x1-a": k8s.SkipOtherOwner},
			wantLists: []string{"0"},
		},
		{
			name: "owners outside the deployment's name do not trigger a re-list",
			objs: []runtime.Object{api, oldRS,
				rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning), foreign},
			want: []string{"api-h1-a"}, wantHash: []string{"h1"},
			wantSkip:  map[string]k8s.SkipReason{"other-x1-a": k8s.SkipOtherOwner},
			wantLists: []string{"0"},
		},
		{
			name: "a failed re-list keeps the first answer", objs: objs, stale: true,
			relistErr: apierrors.NewServiceUnavailable("down"),
			want:      []string{"api-h1-a"}, wantHash: []string{"h1"},
			wantSkip:  map[string]k8s.SkipReason{"api-h2-a": k8s.SkipOtherOwner, "other-x1-a": k8s.SkipOtherOwner},
			wantLists: []string{"0", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(tt.objs...)
			var calls atomic.Int32
			cs.PrependReactor("list", "replicasets", func(ktesting.Action) (bool, runtime.Object, error) {
				if calls.Add(1) > 1 {
					return tt.relistErr != nil, nil, tt.relistErr
				}
				if !tt.stale {
					return false, nil, nil
				}
				return true, &appsv1.ReplicaSetList{Items: []appsv1.ReplicaSet{*oldRS}}, nil
			})

			target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
			if err != nil {
				t.Fatalf("DeploymentLogTarget: %v", err)
			}
			checkTarget(t, target, tt.want, tt.wantHash, tt.wantSkip, len(tt.want), false)

			var lists []string
			for _, call := range apiCalls(cs.Actions()) {
				if call.Resource == "replicasets" {
					lists = append(lists, call.ResourceVersion)
				}
			}
			if !slices.Equal(lists, tt.wantLists) {
				t.Errorf("ReplicaSet list resourceVersions = %q, want %q", lists, tt.wantLists)
			}
		})
	}
}

func TestDeploymentLogTargetErrors(t *testing.T) {
	t.Parallel()

	noSelector := deployment("shop", "api", "uid-api", nil)
	noSelector.Spec.Selector = nil
	emptySelector := deployment("shop", "api", "uid-api", nil)
	badSelector := deployment("shop", "api", "uid-api", nil)
	badSelector.Spec.Selector.MatchExpressions = []metav1.LabelSelectorRequirement{{Key: "tier", Operator: "Near"}}
	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})

	tests := []struct {
		name    string
		objs    []runtime.Object
		react   func(k8stest.Reactor)
		wantErr func(error) bool
	}{
		{name: "missing deployment", wantErr: apierrors.IsNotFound},
		{name: "nil selector", objs: []runtime.Object{noSelector}, wantErr: isNoSelector},
		{name: "empty selector", objs: []runtime.Object{emptySelector}, wantErr: isNoSelector},
		{name: "unparsable selector", objs: []runtime.Object{badSelector}, wantErr: func(err error) bool { return err != nil && !isNoSelector(err) }},
		{
			name: "pods forbidden", objs: []runtime.Object{api},
			react:   func(r k8stest.Reactor) { k8stest.Forbid(r, "list", "pods") },
			wantErr: apierrors.IsForbidden,
		},
		{
			name: "replicaset list fails otherwise", objs: []runtime.Object{api},
			react: func(r k8stest.Reactor) {
				r.PrependReactor("list", "replicasets", func(ktesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewUnauthorized("Unauthorized")
				})
			},
			wantErr: apierrors.IsUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c, cs := k8stest.NewFakeClient(tt.objs...)
			if tt.react != nil {
				tt.react(cs)
			}
			target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
			if !tt.wantErr(err) {
				t.Fatalf("err = %v, not the expected kind", err)
			}
			if len(target.Pods) != 0 {
				t.Errorf("a failed resolve returned pods: %+v", target.Pods)
			}
		})
	}
}

func isNoSelector(err error) bool { return errors.Is(err, k8s.ErrNoSelector) }

func TestDeploymentLogTargetMatchExpressions(t *testing.T) {
	t.Parallel()

	api := deployment("shop", "api", "uid-api", map[string]string{"app": "api"})
	api.Spec.Selector.MatchExpressions = []metav1.LabelSelectorRequirement{
		{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"b", "a"}},
	}
	rs := replicaSet(api, "h1", "uid-h1", "1", 1)
	rs.Labels["tier"] = "a"
	pod := rsPod("shop", "api-h1-a", "api", "h1", "uid-h1", corev1.PodRunning)
	pod.Labels["tier"] = "a"
	c, _ := k8stest.NewFakeClient(api, rs, pod)

	target, err := c.DeploymentLogTarget(context.Background(), "shop", "api")
	if err != nil {
		t.Fatalf("DeploymentLogTarget: %v", err)
	}
	if want := "app=api,tier in (a,b)"; target.Selector != want {
		t.Errorf("Selector = %q, want %q", target.Selector, want)
	}
	if got := footer(t, target, 10).String(); got != "kubectl logs -f -l 'app=api,tier in (a,b),pod-template-hash in (h1)' --prefix --tail=10 -n shop" {
		t.Errorf("footer = %s", got)
	}
}
