package dashboard_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// The deployment rows of busyK3s's panel 4, by their footer command.
const (
	workerRow = "kubectl describe deployment payments-worker -n shop"
	apiRow    = "kubectl describe deployment api -n staging"
	lokiRow   = "kubectl describe statefulset loki-backend -n monitoring"
)

// ownedDeployment is a deployment as the controllers leave it: its
// ReplicaSet of hash h1 and pods running containers, each controlled by
// that ReplicaSet, with init as their init containers.
func ownedDeployment(ns, name string, pods int32, init []corev1.Container, containers ...string) []runtime.Object {
	yes := true
	labels := map[string]string{"app": name}
	dep := tuitest.Deployment(ns, name, pods, labels, containers...)
	dep.UID = types.UID("uid-" + name)
	dep.Spec.Template.Spec.InitContainers = init
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: name + "-h1", Namespace: ns, UID: types.UID("uid-rs-" + name),
		Labels:          map[string]string{"app": name, "pod-template-hash": "h1"},
		Annotations:     map[string]string{"deployment.kubernetes.io/revision": "1"},
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, UID: dep.UID, Controller: &yes}},
	}}
	objs := make([]runtime.Object, 0, 2+pods)
	objs = append(objs, dep, rs)
	for i := range pods {
		p := tuitest.Pod(ns, name+"-h1-"+string(rune('a'+i)), map[string]string{"app": name, "pod-template-hash": "h1"}, containers...)
		p.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: &yes}}
		p.Spec.InitContainers = init
		objs = append(objs, p)
	}
	return objs
}

func workerRef() views.ResourceRef {
	return views.ResourceRef{Kind: "deployment", Namespace: "shop", Name: "payments-worker"}
}

// onWorker is the dashboard over busyK3s on a cluster holding objs, with
// the cursor on payments-worker's row.
func onWorker(t *testing.T, objs ...runtime.Object) dashboard.Model {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(objs...))
	return onRow(t, mountOn(t, c, newFeed(busyK3s), wideW, wideH), "4", workerRow)
}

// wantWorkerPicker requires the container picker for payments-worker's two
// pods, its init container labelled.
func wantWorkerPicker(t *testing.T, out []tea.Msg) {
	t.Helper()
	got, ok := tuitest.Find[views.ContainerPromptRequestMsg](out)
	if !ok || got.Namespace != "shop" || got.Target != workerRef() || got.NextKind != "logs" || got.Detached ||
		!slices.Equal(got.Pods, []string{"payments-worker-h1-a", "payments-worker-h1-b"}) ||
		!slices.Equal(got.Containers, []string{"app", "migrate"}) || !slices.Equal(got.Init, []string{"migrate"}) ||
		got.Logs.Kind != "deployment" || got.Logs.Name != "payments-worker" {
		t.Fatalf("picker request = %+v (found %v):\n%s", got, ok, tuitest.Describe(out))
	}
}

// wantWorkerTail requires the tail prompt for payments-worker's one pod.
func wantWorkerTail(t *testing.T, out []tea.Msg) {
	t.Helper()
	got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
	if !ok || got.Container != "app" || got.Target != workerRef() || !slices.Equal(got.Pods, []string{"payments-worker-h1-a"}) {
		t.Fatalf("tail request = %+v (found %v):\n%s", got, ok, tuitest.Describe(out))
	}
}

// TestLogsOnADeploymentRow: l on a deployment row resolves the pods it
// owns and opens the tail prompt, or the container picker when they run
// several containers, init containers included; a deployment that is
// gone, scaled to zero or without pods says so.
func TestLogsOnADeploymentRow(t *testing.T) {
	t.Parallel()

	migrate := []corev1.Container{{Name: "migrate"}}
	cases := []struct {
		name   string
		objs   []runtime.Object
		check  func(t *testing.T, out []tea.Msg)
		status string
	}{
		{
			name: "an init container opens the picker", objs: ownedDeployment("shop", "payments-worker", 2, migrate, "app"),
			check: wantWorkerPicker,
		},
		{
			name: "one container opens the tail prompt", objs: ownedDeployment("shop", "payments-worker", 1, nil, "app"),
			check: wantWorkerTail,
		},
		{name: "a deployment that is gone", status: "deploy shop/payments-worker is gone"},
		{
			name:   "a deployment scaled to zero",
			objs:   []runtime.Object{tuitest.Deployment("shop", "payments-worker", 0, map[string]string{"app": "payments-worker"}, "app")},
			status: "deploy shop/payments-worker has 0 replicas",
		},
		{
			name:   "a deployment without pods",
			objs:   []runtime.Object{tuitest.Deployment("shop", "payments-worker", 2, map[string]string{"app": "payments-worker"}, "app")},
			status: "deploy shop/payments-worker: 0 running pods",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := onWorker(t, tc.objs...)
			if got := hint(m, "l"); got != "logs" {
				t.Errorf("l hint on a deployment row = %q, want logs", got)
			}
			_, out := press(t, m, "l")
			if tc.check != nil {
				tc.check(t, out)
				return
			}
			if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || msg.Text != tc.status {
				t.Fatalf("status = %q (found %v), want %q:\n%s", msg.Text, ok, tc.status, tuitest.Describe(out))
			}
		})
	}
}

// TestLogsOnOtherWorkloadRows: a statefulset, job or cronjob row has no
// logs of its own in k4s: l says what it reads instead, and the footer
// offers no l.
func TestLogsOnOtherWorkloadRows(t *testing.T) {
	t.Parallel()

	m := onRow(t, mount(t, newFeed(busyK3s), wideW, wideH), "4", lokiRow)
	if got := hint(m, "l"); got != "" {
		t.Errorf("l hint on a statefulset row = %q, want none", got)
	}
	_, out := press(t, m, "l")
	if msg, ok := tuitest.Find[views.StatusMsg](out); !ok || msg.Text != "no logs here: l reads the logs of a pod or deployment row" {
		t.Fatalf("status = %q (found %v):\n%s", msg.Text, ok, tuitest.Describe(out))
	}
}

// TestDeploymentLogsLastWins: l on payments-worker, then l on api before
// the first answers: only api's prompt opens, whichever reply lands first.
func TestDeploymentLogsLastWins(t *testing.T) {
	t.Parallel()

	objs := slices.Concat(ownedDeployment("shop", "payments-worker", 1, nil, "app"), ownedDeployment("staging", "api", 1, nil, "app"))
	m := onWorker(t, objs...)
	m, first := tuitest.Step(t, m, tuitest.Press("l"))
	m = onRow(t, m, "4", apiRow)
	m, second := tuitest.Step(t, m, tuitest.Press("l"))
	for _, order := range [][]tea.Msg{slices.Concat(first, second), slices.Concat(second, first)} {
		_, out := tuitest.Drive(t, m, order...)
		reqs := tuitest.All[views.TailPromptRequestMsg](out)
		if len(reqs) != 1 || reqs[0].Target != (views.ResourceRef{Kind: "deployment", Namespace: "staging", Name: "api"}) {
			t.Errorf("want only api's prompt, got:\n%s", tuitest.Describe(tuitest.Requests(out)))
		}
	}
}

// TestDeploymentLogsSurviveAReorder: a poll that re-sorts the rows while
// the resolve runs leaves the cursor on its deployment, and the prompt
// opens for the deployment l was pressed on.
func TestDeploymentLogsSurviveAReorder(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(ownedDeployment("shop", "payments-worker", 1, nil, "app")...))
	m := onRow(t, mountOn(t, c, f, wideW, wideH), "4", workerRow)
	m, reply := tuitest.Step(t, m, tuitest.Press("l"))
	f.set(func() dash.Snapshot {
		s := busyK3s()
		rows := slices.Clone(s.Workloads.Rows)
		slices.Reverse(rows)
		s.Workloads.Rows = rows
		return s
	})
	m = advance(t, m, f, tickStep)
	if got := command(m); got != workerRow {
		t.Fatalf("the cursor left payments-worker after the poll: %q", got)
	}
	_, out := tuitest.Drive(t, m, reply...)
	got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
	if !ok || got.Target != workerRef() || got.Detached {
		t.Fatalf("prompt after the reorder = %+v (found %v):\n%s", got, ok, tuitest.Describe(out))
	}
	if strings.Contains(tuitest.Describe(out), "staging") {
		t.Errorf("another deployment's logs were asked for:\n%s", tuitest.Describe(out))
	}
}
