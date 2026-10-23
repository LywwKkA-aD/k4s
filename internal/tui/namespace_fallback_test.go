package tui_test

import (
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// clusterWideDenied is a fake client whose RBAC refuses listing resource
// across every namespace and whose kubeconfig context sets demo; asked
// counts the refused lists.
func clusterWideDenied(t *testing.T, resource string, asked *atomic.Int32) *k8s.Client {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithReactor("list", resource, func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() != "" {
			return false, nil, nil
		}
		asked.Add(1)
		denial := errors.New(`User "u" cannot list resource "` + resource + `" in API group "" at the cluster scope`)
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: resource}, "", denial)
	}))
	c.ContextNamespace = tuitest.NamespaceDemo
	return c
}

// metricsDeniedClusterWide is a metrics API that refuses pods across every
// namespace and serves demo's; asked counts the refusals.
func metricsDeniedClusterWide(t *testing.T, asked *atomic.Int32) *k8s.Client {
	t.Helper()
	pods := []k8stest.PodMetrics{{
		Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1,
		Containers: []k8stest.ContainerUsage{{Name: tuitest.ContainerNginx, CPU: "12m", Memory: "20Mi"}},
	}}
	denied := k8stest.Forbidden()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /apis/metrics.k8s.io/v1beta1/pods", func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		denied.ServeHTTP(w, r)
	})
	mux.Handle("/", k8stest.Metrics(pods, nil))
	c := k8stest.NewServerClient(t, mux)
	c.ContextNamespace = tuitest.NamespaceDemo
	return c
}

// TestFallbackNamesTheNamespaceInTheRoot: a list that falls back from
// every namespace to the context's is in that namespace for the root too.
// The header says so, and Esc back to the list asks for it again, not for
// every namespace.
func TestFallbackNamesTheNamespaceInTheRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		view   string
		client func(*testing.T, *atomic.Int32) *k8s.Client
		// away leaves the list for a view Esc comes back from.
		away []tea.Msg
		// command is the list's command row once back.
		command string
	}{
		{
			view:   "pods",
			client: func(t *testing.T, n *atomic.Int32) *k8s.Client { return clusterWideDenied(t, "pods", n) },
			away:   []tea.Msg{tuitest.Enter()}, command: "kubectl get pods -n demo",
		},
		{
			view:   "deployments",
			client: func(t *testing.T, n *atomic.Int32) *k8s.Client { return clusterWideDenied(t, "deployments", n) },
			away:   []tea.Msg{tuitest.Enter()}, command: "kubectl get deployments -n demo",
		},
		{
			view:   "services",
			client: func(t *testing.T, n *atomic.Int32) *k8s.Client { return clusterWideDenied(t, "services", n) },
			away:   []tea.Msg{tuitest.Enter()}, command: "kubectl get services -n demo",
		},
		{
			view:   "events",
			client: func(t *testing.T, n *atomic.Int32) *k8s.Client { return clusterWideDenied(t, "events", n) },
			away:   colon("svc"), command: "kubectl events -n demo",
		},
		{
			view: "top", client: metricsDeniedClusterWide,
			away: colon("svc"), command: "top pods -n demo",
		},
	}
	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			t.Parallel()
			var asked atomic.Int32
			m := started(t, tc.client(t, &asked))
			m, out := tuitest.Drive(t, m, colon(tc.view)...)
			requireView(t, m, tc.view)
			if got := tuitest.HeaderNamespace(m); got != tuitest.NamespaceDemo {
				t.Fatalf("header namespace after the fallback = %q, want demo:\n%s", got, tuitest.Screen(m))
			}
			if got := tuitest.All[views.NamespaceFallbackMsg](out); len(got) != 1 || got[0].View != tc.view {
				t.Errorf("fallback messages = %+v, want one from %s", got, tc.view)
			}
			// The dashboard may have asked too; what counts is that Esc
			// does not.
			before := asked.Load()

			m, _ = tuitest.Drive(t, m, tc.away...)
			if tuitest.HeaderView(m) == tc.view {
				t.Fatalf("still on %s after leaving it:\n%s", tc.view, tuitest.Screen(m))
			}
			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			requireView(t, m, tc.view)
			if got := asked.Load() - before; got != 0 {
				t.Errorf("Esc asked for every namespace %d more times, want none", got)
			}
			if got := tuitest.HeaderNamespace(m); got != tuitest.NamespaceDemo {
				t.Errorf("header namespace after Esc = %q, want demo", got)
			}
			if got := commandRow(m); !strings.Contains(got, tc.command) {
				t.Errorf("command row = %q, want %q in it", got, tc.command)
			}
		})
	}
}

// TestFallbackFromAnotherViewIsIgnored: a fallback for a list that is
// neither on screen nor in the history, or that the root has since moved
// to a namespace, changes nothing.
func TestFallbackFromAnotherViewIsIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		setup []tea.Msg
		// view is the list the fallback is from.
		view string
		want string
	}{
		{name: "a list never opened", setup: colon("ctx"), view: "pods", want: "ALL"},
		{name: "already in a namespace", setup: colon("ns shop"), view: "pods", want: "shop"},
		{name: "not a namespaced list", setup: colon("no"), view: "nodes", want: "ALL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), tc.setup...)
			m, _ = tuitest.Drive(t, m, views.NamespaceFallbackMsg{View: tc.view, Namespace: tuitest.NamespaceDemo})
			if got := tuitest.HeaderNamespace(m); got != tc.want {
				t.Errorf("header namespace = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFallbackAfterLeavingTheList: a fallback that lands after the user
// left the list (a describe opened on a row, another list typed) goes to
// the list's history entry, so Esc lists the namespace and not every one
// again; the view on screen keeps its own namespace.
func TestFallbackAfterLeavingTheList(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		away []tea.Msg
		// stays is the command row of the view left on screen.
		stays string
	}{
		{name: "a describe opened on a row", away: []tea.Msg{tuitest.Enter()}, stays: "kubectl describe pod"},
		{name: "another list", away: colon("svc"), stays: "kubectl get services -A"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), then(colon("pods"), tc.away)...)
			if tuitest.HeaderView(m) == "pods" {
				t.Fatalf("still on pods after leaving it:\n%s", tuitest.Screen(m))
			}
			m, _ = tuitest.Drive(t, m, views.NamespaceFallbackMsg{View: "pods", Namespace: tuitest.NamespaceDemo})
			if got := commandRow(m); !strings.Contains(got, tc.stays) {
				t.Errorf("command row after the fallback = %q, want %q in it", got, tc.stays)
			}

			m, _ = tuitest.Drive(t, m, tuitest.Esc())
			requireView(t, m, "pods")
			if got := tuitest.HeaderNamespace(m); got != tuitest.NamespaceDemo {
				t.Errorf("header namespace after Esc = %q, want demo", got)
			}
			if got := commandRow(m); !strings.Contains(got, "kubectl get pods -n demo") {
				t.Errorf("command row after Esc = %q, want the pods of demo", got)
			}
		})
	}
}
