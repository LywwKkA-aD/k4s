package dashboard_test

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

const r8vqn = "payments-worker-6c9f7d8b5-r8vqn"

// unplaced is the busy cluster with r8vqn's namespace not looked up yet:
// the data layer finds it by name.
func unplaced() dash.Snapshot {
	s := busyK3s()
	rows := append([]dash.PodRow(nil), s.Pods.Problems...)
	for i, r := range rows {
		if r.Key.Name == r8vqn {
			r.Key.Namespace, r.Resolved = "", false
			r.Cmd = kc("get", "pods", "-A", "--field-selector", "metadata.name="+r8vqn)
			rows[i] = r
		}
	}
	s.Pods.Problems = rows
	return s
}

// ringRow focuses panel 5 and steps down to r8vqn's row in the ring, the
// one under the cursor that names a change.
func ringRow(t *testing.T, m dashboard.Model) dashboard.Model {
	t.Helper()
	m, _ = press(t, m, "5")
	for range 16 {
		m, _ = press(t, m, "down")
		for _, l := range strings.Split(tuitest.Screen(m), "\n") {
			if strings.Contains(l, "›") && strings.Contains(l, r8vqn) && strings.Contains(l, "→") {
				return m
			}
		}
	}
	t.Fatalf("no ring row for %s:\n%s", r8vqn, tuitest.Screen(m))
	return m
}

// TestRingRowsCarryTheDataLayersCommand: a change to a pod whose
// namespace is not known yet finds it by name, as its panel row does, and
// once a later poll places it the ring describes it in its namespace.
func TestRingRowsCarryTheDataLayersCommand(t *testing.T) {
	t.Parallel()

	f := newFeed(calm)
	m := mount(t, f, wideW, wideH)
	f.set(unplaced)
	m = advance(t, m, f, 5*time.Second)
	m = ringRow(t, m)
	if got, want := command(m), "kubectl get pods -A --field-selector metadata.name="+r8vqn; got != want {
		t.Fatalf("unplaced: command = %q, want %q", got, want)
	}
	f.set(busyK3s)
	m = advance(t, m, f, 5*time.Second)
	m = ringRow(t, m)
	if got, want := command(m), "kubectl describe pod "+r8vqn+" -n shop"; got != want {
		t.Fatalf("placed: command = %q, want %q", got, want)
	}
}

// nodesOnly is a cluster k4s reads through typed lists alone: nodes a and
// b, no Tables.
func nodesOnly(t *testing.T) (*fake.Clientset, dashboard.FetchFunc, *feed) {
	t.Helper()
	ready := corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}
	cs := fake.NewClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-a"}, Status: ready},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-b"}, Status: ready},
	)
	f := newFeed(nil)
	fetch := dash.NewFetcher(dash.Source{Typed: cs}).WithClock(f.clock.Now).Fetch
	return cs, fetch, f
}

// TestAGoneObjectHasNoCommand: a node that left the cluster stays in the
// ring as gone, with no command to copy, and Enter and l on it say so
// instead of opening anything.
func TestAGoneObjectHasNoCommand(t *testing.T) {
	t.Parallel()

	cs, fetch, f := nodesOnly(t)
	opts := append(f.options(), dashboard.WithFetch(fetch))
	m, _ := tuitest.Mount(t, dashboard.New(tuitest.NewFakeClient(t), nil, opts...), wideW, wideH)
	if err := cs.CoreV1().Nodes().Delete(context.Background(), "node-b", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	m = advance(t, m, f, 30*time.Second)
	if !strings.Contains(tuitest.Screen(m), "node node-b  Ready → gone") {
		t.Fatalf("the ring lacks node-b's departure:\n%s", tuitest.Screen(m))
	}
	m, _ = press(t, m, "5", "down")
	if got := command(m); strings.Contains(got, "node-b") {
		t.Fatalf("a gone node still has a command: %q", got)
	}
	for _, k := range []string{"enter", "l"} {
		_, out := press(t, m, k)
		msg, ok := tuitest.Find[views.StatusMsg](out)
		_, described := tuitest.Find[views.DescribeRequestMsg](out)
		_, tailed := tuitest.Find[views.TailPromptRequestMsg](out)
		if !ok || msg.Text != "node node-b is gone" || described || tailed {
			t.Fatalf("%s on a gone node:\n%s", k, tuitest.Describe(out))
		}
	}
}
