package deployments_test

import (
	"context"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// hostileWeb is the raw name of the one hostile deployment among clean
// ones. It sorts after them (U+202E is past ASCII), so it is never raw
// index 0.
var hostileWeb = tuitest.Hostile("pay-web")

// ownedAs is owned for a deployment named name whose labels, ReplicaSet
// and one pod are named after app, so a hostile name stays out of the
// selector: <app>-h1 runs <app>-h1-a.
func ownedAs(name, app string) []runtime.Object {
	labels := map[string]string{"app": app}
	dep := tuitest.Deployment(shop, name, 1, labels, appContainer)
	dep.UID = types.UID("uid-" + app)
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
		Name: app + "-" + hash, Namespace: shop, UID: types.UID("uid-rs-" + app),
		Labels:          map[string]string{"app": app, hashKey: hash},
		Annotations:     map[string]string{revision: "1"},
		OwnerReferences: []metav1.OwnerReference{controllerRef("Deployment", name, dep.UID)},
	}}
	pod := tuitest.Pod(shop, podName(app, 0), map[string]string{"app": app, hashKey: hash}, appContainer)
	pod.OwnerReferences = []metav1.OwnerReference{controllerRef("ReplicaSet", rs.Name, rs.UID)}
	return []runtime.Object{dep, rs, pod}
}

// mixedDeployments are api, pay-db and the hostile pay-web in shop, in
// that order, each running one pod.
func mixedDeployments(t *testing.T) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, tuitest.WithObjects(slices.Concat(
		[]runtime.Object{tuitest.Namespace(shop)},
		ownedAs("api", "api"), ownedAs("pay-db", "pay-db"), ownedAs(hostileWeb, "pay-web"),
	)...))
}

// cursorOn are the ways to put the cursor on a row whose shown index is
// not its raw one: the raw name and pod each lands on.
var cursorOn = []struct {
	name string
	keys []tea.Msg
	want string
	pod  string
}{
	{name: "a clean row first under a filter", keys: tuitest.FilterBy("pay"), want: "pay-db", pod: podName("pay-db", 0)},
	{name: "the hostile row second under a filter", keys: on("pay", tuitest.Press("down")), want: hostileWeb, pod: podName("pay-web", 0)},
	{name: "the hostile row alone under a filter on its shown name", keys: tuitest.FilterBy("web"), want: hostileWeb, pod: podName("pay-web", 0)},
	{name: "the hostile row last unfiltered", keys: tuitest.Keys("down", "down"), want: hostileWeb, pod: podName("pay-web", 0)},
}

// Every row action targets the raw name of the row under the cursor.
func TestRowActionsTargetTheRawRowUnderTheCursor(t *testing.T) {
	t.Parallel()

	for _, at := range cursorOn {
		cases := []struct {
			key   string
			check func(t *testing.T, msgs []tea.Msg)
		}{
			{key: "enter", check: func(t *testing.T, msgs []tea.Msg) {
				if want := (views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: at.want}); !tuitest.Contains(msgs, want) {
					t.Errorf("enter sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
			{key: "l", check: func(t *testing.T, msgs []tea.Msg) {
				req, ok := tuitest.Find[views.TailPromptRequestMsg](msgs)
				if !ok || req.Target != ref(at.want) || req.Logs.Name != at.want || !slices.Equal(req.Pods, []string{at.pod}) {
					t.Errorf("l sent %s, want the logs of %q (pod %s)", tuitest.Describe(msgs), at.want, at.pod)
				}
			}},
			{key: "f", check: func(t *testing.T, msgs []tea.Msg) {
				if want := (views.ForwardRequestMsg{Kind: "deployment", Namespace: shop, Name: at.want}); !tuitest.Contains(msgs, want) {
					t.Errorf("f sent %s, want %+v", tuitest.Describe(msgs), want)
				}
			}},
		}
		for _, tc := range cases {
			t.Run(at.name+" "+tc.key, func(t *testing.T) {
				t.Parallel()
				m, _ := tuitest.Drive(t, mounted(t, mixedDeployments(t), shop), at.keys...)
				if got, ok := m.Selected(); !ok || got != ref(at.want) {
					t.Fatalf("Selected() = %+v, %v, want %q", got, ok, at.want)
				}
				_, msgs := tuitest.Drive(t, m, tuitest.Press(tc.key))
				tc.check(t, msgs)
			})
		}
	}
}

// A refresh that puts a new deployment above the hostile row keeps the
// cursor on the same raw deployment, not on the index it had.
func TestRefreshKeepsTheCursorOnTheRawDeployment(t *testing.T) {
	t.Parallel()

	c := mixedDeployments(t)
	m, _ := tuitest.Drive(t, mounted(t, c, shop), tuitest.Keys("down", "down")...)
	dep := tuitest.Deployment(shop, "aaa", 1, map[string]string{"app": "aaa"}, appContainer)
	if _, err := c.Clientset.AppsV1().Deployments(shop).Create(context.Background(), dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got, ok := m.Selected(); !ok || got != ref(hostileWeb) {
		t.Fatalf("after the refresh Selected() = %+v, %v, want the hostile deployment", got, ok)
	}
	if got := m.State().Cursor; got != 3 {
		t.Errorf("cursor index = %d, want 3 (aaa, api, pay-db, then it)", got)
	}
}
