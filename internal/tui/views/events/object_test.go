package events_test

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// TestEnterOnAnObjectWithoutANamespace: an event's involvedObject may
// leave its namespace out. A namespaced kind lives where the event does,
// a cluster-scoped one is described without -n, and a kind k4s does not
// know cannot be placed: the status says so instead of describing it in
// the context's namespace, and names the kubectl describe that works if
// the kind is cluster-scoped, as a custom resource such as cert-manager's
// ClusterIssuer is.
func TestEnterOnAnObjectWithoutANamespace(t *testing.T) {
	t.Parallel()

	ref := func(apiVersion, kind, name string) corev1.ObjectReference {
		return corev1.ObjectReference{APIVersion: apiVersion, Kind: kind, Name: name}
	}
	cases := []struct {
		name   string
		obj    corev1.ObjectReference
		want   views.DescribeRequestMsg
		status string
	}{
		{name: "pod", obj: ref("v1", "Pod", "web-1"), want: views.DescribeRequestMsg{Kind: "pod", Namespace: "kube-system", Name: "web-1"}},
		{name: "deployment", obj: ref("apps/v1", "Deployment", "web"), want: views.DescribeRequestMsg{Kind: "deployment", Namespace: "kube-system", Name: "web"}},
		{name: "k3s helm chart", obj: ref("helm.cattle.io/v1", "HelmChart", "traefik"), want: views.DescribeRequestMsg{Kind: "helmchart.helm.cattle.io", Namespace: "kube-system", Name: "traefik"}},
		{name: "node", obj: ref("v1", "Node", "k3s-server"), want: views.DescribeRequestMsg{Kind: "node", Name: "k3s-server"}},
		{name: "storage class", obj: ref("storage.k8s.io/v1", "StorageClass", "local-path"), want: views.DescribeRequestMsg{Kind: "storageclass.storage.k8s.io", Name: "local-path"}},
		{name: "custom resource", obj: ref("example.com/v1", "Widget", "w1"),
			status: "event odd.1 names Widget/w1 without a namespace; if it is cluster-scoped: kubectl describe widget.example.com w1"},
		{name: "a built-in name in another group", obj: ref("example.com/v1", "Pod", "p1"),
			status: "event odd.1 names Pod/p1 without a namespace; if it is cluster-scoped: kubectl describe pod.example.com p1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			e := ev("kube-system", "odd.1", "Normal", "Odd", "odd", tc.obj, 1, time.Hour, time.Hour)
			c := tuitest.NewFakeClient(t, tuitest.WithObjects(e))
			c.ContextNamespace = demo
			m, _ := mounted(t, c, "")
			_, out := tuitest.Drive(t, m, tuitest.Enter())
			got, described := tuitest.Find[views.DescribeRequestMsg](out)
			if tc.status != "" {
				if described {
					t.Fatalf("Enter described %+v", got)
				}
				requireStatus(t, out, status.Warn, tc.status)
				return
			}
			if !described || got != tc.want {
				t.Errorf("Enter asked %+v, want %+v:\n%s", got, tc.want, tuitest.Describe(out))
			}
		})
	}
}
