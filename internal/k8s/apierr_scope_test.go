package k8s_test

import (
	"context"
	"errors"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// clusterScoped are the built-in resources that live outside namespaces.
var clusterScoped = []struct{ resource, group string }{
	{"nodes", ""}, {"namespaces", ""}, {"persistentvolumes", ""},
	{"storageclasses", "storage.k8s.io"}, {"csidrivers", "storage.k8s.io"},
	{"csinodes", "storage.k8s.io"}, {"volumeattachments", "storage.k8s.io"},
	{"clusterroles", "rbac.authorization.k8s.io"}, {"clusterrolebindings", "rbac.authorization.k8s.io"},
	{"customresourcedefinitions", "apiextensions.k8s.io"}, {"priorityclasses", "scheduling.k8s.io"},
	{"ingressclasses", "networking.k8s.io"}, {"runtimeclasses", "node.k8s.io"},
	{"apiservices", "apiregistration.k8s.io"},
	{"mutatingwebhookconfigurations", "admissionregistration.k8s.io"},
	{"validatingwebhookconfigurations", "admissionregistration.k8s.io"},
	{"certificatesigningrequests", "certificates.k8s.io"},
}

// TestClassifyClusterScoped: a resource that lives outside namespaces is
// asked about without -A, and its refusal is not worded as a list across
// every namespace, whether the call or only the server names it.
func TestClassifyClusterScoped(t *testing.T) {
	t.Parallel()

	for _, r := range clusterScoped {
		t.Run(r.resource, func(t *testing.T) {
			t.Parallel()
			name := r.resource
			if r.group != "" {
				name += "." + r.group
			}
			msg := `list resource "` + r.resource + `" in API group "` + r.group + `" at the cluster scope`
			for _, tc := range []struct {
				err  error
				want string // the resource as the refusal names it
			}{
				{err: forbidden(r.resource, r.group, msg), want: name},
				{err: listCall(name, "", forbidden(r.resource, r.group, msg)), want: name},
				// A call that does not say it is cluster-scoped.
				{err: listCall(r.resource, "", forbidden(r.resource, r.group, "no")), want: r.resource},
			} {
				got := k8s.ClassifyOnHost(tc.err, false)
				if got.Short != "forbidden: list "+tc.want || got.Hint != "kubectl auth can-i list "+tc.want || got.AllNamespaces {
					t.Errorf("Classify(%v) = %q %q (all namespaces %v)", tc.err, got.Short, got.Hint, got.AllNamespaces)
				}
			}
		})
	}
}

func TestKindScope(t *testing.T) {
	t.Parallel()

	cases := []struct {
		group, kind string
		want        k8s.Scope
	}{
		{kind: "Node", want: k8s.ScopeCluster},
		{group: "storage.k8s.io", kind: "StorageClass", want: k8s.ScopeCluster},
		{group: "k3s.cattle.io", kind: "ETCDSnapshotFile", want: k8s.ScopeCluster},
		{kind: "Pod", want: k8s.ScopeNamespaced},
		{group: "apps", kind: "Deployment", want: k8s.ScopeNamespaced},
		{group: "events.k8s.io", kind: "Event", want: k8s.ScopeNamespaced},
		{group: "helm.cattle.io", kind: "HelmChart", want: k8s.ScopeNamespaced},
		{group: "example.com", kind: "Pod", want: k8s.ScopeUnknown},
		{group: "example.com", kind: "Widget", want: k8s.ScopeUnknown},
		{kind: "StorageClass", want: k8s.ScopeUnknown},
	}
	for _, tc := range cases {
		if got := k8s.KindScope(tc.group, tc.kind); got != tc.want {
			t.Errorf("KindScope(%q, %q) = %v, want %v", tc.group, tc.kind, got, tc.want)
		}
	}
}

// TestEventAndNodeListsNameTheirScope: a refused list of events or nodes
// is classified from the call, as RBAC names it: events across every
// namespace (the views fall back on that) and nodes, which have none.
func TestEventAndNodeListsNameTheirScope(t *testing.T) {
	t.Parallel()

	c, cs := k8stest.NewFakeClient()
	k8stest.RestrictToNamespace(cs, "demo")
	ctx := context.Background()
	_, evErr := c.ListEvents(ctx, "")
	_, otherErr := c.ListEvents(ctx, "other")
	_, nodeErr := c.ListNodes(ctx)

	cases := []struct {
		name string
		err  error
		text string
		all  bool
	}{
		{name: "events, all namespaces", err: evErr, text: "forbidden: list events in all namespaces · kubectl auth can-i list events -A", all: true},
		{name: "events, another namespace", err: otherErr, text: "forbidden: list events in other · kubectl auth can-i list events -n other"},
		{name: "nodes", err: nodeErr, text: "forbidden: list nodes · kubectl auth can-i list nodes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var call *k8s.CallError
			if !errors.As(tc.err, &call) {
				t.Fatalf("err = %v, want a *k8s.CallError", tc.err)
			}
			d := k8s.ClassifyOnHost(tc.err, false)
			if d.Text() != tc.text || d.AllNamespaces != tc.all {
				t.Errorf("Classify = %q (all namespaces %v), want %q (%v)", d.Text(), d.AllNamespaces, tc.text, tc.all)
			}
		})
	}
}

// TestClassifyClusterScopedFromTheServer: what the server alone says about
// a cluster-scoped request, a subresource included; a custom resource that
// only shares a built-in name keeps -A, which is harmless for either scope.
func TestClassifyClusterScopedFromTheServer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, msg, short, hint string
	}{
		{
			name:  "one node",
			msg:   `get resource "nodes" in API group "" at the cluster scope`,
			short: "forbidden: get nodes",
			hint:  "kubectl auth can-i get nodes",
		},
		{
			name:  "a node's subresource",
			msg:   `get resource "nodes/proxy" in API group "" at the cluster scope`,
			short: "forbidden: get nodes/proxy",
			hint:  "kubectl auth can-i get nodes --subresource=proxy",
		},
		{
			name:  "a custom resource that shares a built-in name",
			msg:   `list resource "nodes" in API group "example.com" at the cluster scope`,
			short: "forbidden: list nodes.example.com at cluster scope",
			hint:  "kubectl auth can-i list nodes.example.com -A",
		},
		{
			name:  "a namespaced resource across every namespace",
			msg:   `list resource "events" in API group "" at the cluster scope`,
			short: "forbidden: list events at cluster scope",
			hint:  "kubectl auth can-i list events -A",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := k8s.ClassifyOnHost(apierrors.NewForbidden(schema.GroupResource{}, "", errors.New(denied+tc.msg)), false)
			if got.Short != tc.short || got.Hint != tc.hint {
				t.Errorf("Classify = %q %q, want %q %q", got.Short, got.Hint, tc.short, tc.hint)
			}
		})
	}
}
