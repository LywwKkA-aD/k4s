package pods_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

// denyAllNamespaces makes RBAC refuse a pod list across every namespace,
// worded as the apiserver words it; lists in one namespace go through.
func denyAllNamespaces() tuitest.FakeOption {
	return tuitest.WithReactor("list", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() != "" {
			return false, nil, nil
		}
		denial := errors.New(`User "u" cannot list resource "pods" in API group "" at the cluster scope`)
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", denial)
	})
}

// scopedClient is a fake client whose kubeconfig context sets namespace.
func scopedClient(t *testing.T, namespace string, opts ...tuitest.FakeOption) *k8s.Client {
	t.Helper()
	c := tuitest.NewFakeClient(t, opts...)
	c.ContextNamespace = namespace
	return c
}

func statusesAt(msgs []tea.Msg, level status.Level) []string {
	var out []string
	for _, s := range tuitest.All[views.StatusMsg](msgs) {
		if s.Level == level {
			out = append(out, s.Text)
		}
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// TestForbiddenClusterWideLandsInTheContextNamespace is M4.3's fallback:
// the rows of the context's namespace, its command and a hint.
func TestForbiddenClusterWideLandsInTheContextNamespace(t *testing.T) {
	t.Parallel()

	c := scopedClient(t, tuitest.NamespaceDemo, denyAllNamespaces())
	m, out := tuitest.Mount(t, pods.New(c, ""), width, height)

	want := "cluster-wide list forbidden; showing demo · kubectl auth can-i list pods -A"
	if got := statusesAt(out, status.Warn); len(got) != 1 || got[0] != want {
		t.Errorf("warnings = %q, want [%q]", got, want)
	}
	if got := statusesAt(out, status.Error); len(got) != 0 {
		t.Errorf("errors = %q, want none", got)
	}
	screen := tuitest.Screen(m)
	for _, w := range []string{tuitest.PodWeb1, tuitest.PodCrash} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	if strings.Contains(screen, "NAMESPACE") || strings.Contains(screen, tuitest.PodCoreDNS) {
		t.Errorf("the fallback still shows every namespace:\n%s", screen)
	}
	if got := m.Command().String(); got != "kubectl get pods -n demo" {
		t.Errorf("Command = %q", got)
	}

	// It keeps listing the context namespace on refresh.
	m, out = tuitest.Drive(t, m, tuitest.Press("r"))
	if got := tuitest.All[views.StatusMsg](out); len(got) != 0 || !strings.Contains(tuitest.Screen(m), tuitest.PodWeb1) {
		t.Errorf("r after the fallback said %v:\n%s", got, tuitest.Screen(m))
	}
}

// TestForbiddenClusterWideWithoutAContextNamespace: nothing to fall back
// to, so the error body and the status point at :ns.
func TestForbiddenClusterWideWithoutAContextNamespace(t *testing.T) {
	t.Parallel()

	m, out := tuitest.Mount(t, pods.New(scopedClient(t, "", denyAllNamespaces()), ""), width, height)
	want := "forbidden: list pods in all namespaces · :ns <name> picks one · kubectl auth can-i list pods -A"
	if got := statusesAt(out, status.Error); len(got) != 1 || got[0] != want {
		t.Errorf("errors = %q, want [%q]", got, want)
	}
	if screen := oneLine(tuitest.Screen(m)); !strings.Contains(screen, "pods unavailable: "+want) {
		t.Errorf("body lacks the classified error:\n%s", screen)
	}
	if got := m.Command().String(); got != "kubectl get pods -A" {
		t.Errorf("Command = %q", got)
	}
}

// TestForbiddenInTheContextNamespaceToo: the fallback happens once; a
// refusal there is an error like any other, with its own can-i.
func TestForbiddenInTheContextNamespaceToo(t *testing.T) {
	t.Parallel()

	deny := tuitest.WithReactor("list", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("no"))
	})
	m, out := tuitest.Mount(t, pods.New(scopedClient(t, tuitest.NamespaceDemo, deny), ""), width, height)
	want := "forbidden: list pods in demo · kubectl auth can-i list pods -n demo"
	if got := statusesAt(out, status.Error); len(got) != 1 || got[0] != want {
		t.Errorf("errors = %q, want [%q]", got, want)
	}
	if screen := oneLine(tuitest.Screen(m)); !strings.Contains(screen, "pods unavailable: "+want) {
		t.Errorf("body:\n%s", screen)
	}
}
