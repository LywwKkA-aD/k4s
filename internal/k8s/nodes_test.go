package k8s

import (
	"context"
	"errors"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// fakeNode is a node with the given labels, Ready condition (nil for
// none), kubelet version and taints, created two days ago.
func fakeNode(name string, labels map[string]string, ready *corev1.ConditionStatus, version string, taints int) *corev1.Node {
	n := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels, CreationTimestamp: metav1.NewTime(time.Now().Add(-49 * time.Hour))},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{KubeletVersion: version, OperatingSystem: "linux", Architecture: "arm64"},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeHostName, Address: name},
				{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
			},
		},
	}
	if ready != nil {
		n.Status.Conditions = []corev1.NodeCondition{
			{Type: corev1.NodeMemoryPressure, Status: corev1.ConditionFalse},
			{Type: corev1.NodeReady, Status: *ready},
		}
	}
	for i := range taints {
		n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{Key: "t" + string(rune('a'+i)), Effect: corev1.TaintEffectNoSchedule})
	}
	return n
}

func condition(s corev1.ConditionStatus) *corev1.ConditionStatus { return &s }

const rolePrefix = "node-role.kubernetes.io/"

// TestListNodesAsKubectlGetNodesWide pins STATUS, ROLES, VERSION,
// INTERNAL-IP and OS/ARCH as kubectl prints them, on the nodes a k3s
// cluster has: servers with and without embedded etcd, agents, cordoned
// and not-ready nodes, and kubelets of different versions mid-upgrade.
func TestListNodesAsKubectlGetNodesWide(t *testing.T) {
	t.Parallel()

	ready, notReady, unknown := condition(corev1.ConditionTrue), condition(corev1.ConditionFalse), condition(corev1.ConditionUnknown)
	cordoned := fakeNode("e-cordoned", nil, ready, "v1.35.2+k3s1", 1)
	cordoned.Spec.Unschedulable = true
	bare := fakeNode("g-bare", map[string]string{rolePrefix: "", "kubernetes.io/role": ""}, nil, "v1.36.5+k3s1", 0)
	bare.Status.Addresses = nil
	bare.Status.NodeInfo.OperatingSystem, bare.Status.NodeInfo.Architecture = "", ""

	cases := []struct {
		node *corev1.Node
		want Node
	}{
		{
			node: fakeNode("a-server", map[string]string{rolePrefix + "control-plane": "true", rolePrefix + "master": "true"}, ready, "v1.36.5+k3s1", 0),
			want: Node{Name: "a-server", Status: "Ready", Roles: "control-plane,master", Version: "v1.36.5+k3s1", InternalIP: "192.168.1.10", OSArch: "linux/arm64"},
		},
		{
			node: fakeNode("b-etcd", map[string]string{rolePrefix + "master": "true", rolePrefix + "etcd": "true", rolePrefix + "control-plane": "true"}, ready, "v1.36.5+k3s1", 2),
			want: Node{Name: "b-etcd", Status: "Ready", Roles: "control-plane,etcd,master", Version: "v1.36.5+k3s1", InternalIP: "192.168.1.10", OSArch: "linux/arm64", Taints: 2},
		},
		{
			node: fakeNode("c-agent", map[string]string{"kubernetes.io/hostname": "c-agent"}, ready, "v1.35.2+k3s1", 0),
			want: Node{Name: "c-agent", Status: "Ready", Roles: "<none>", Version: "v1.35.2+k3s1", InternalIP: "192.168.1.10", OSArch: "linux/arm64"},
		},
		{
			node: fakeNode("d-worker", map[string]string{"kubernetes.io/role": "worker"}, notReady, "v1.34.1", 0),
			want: Node{Name: "d-worker", Status: "NotReady", Roles: "worker", Version: "v1.34.1", InternalIP: "192.168.1.10", OSArch: "linux/arm64"},
		},
		{
			node: cordoned,
			want: Node{Name: "e-cordoned", Status: "Ready,SchedulingDisabled", Roles: "<none>", Version: "v1.35.2+k3s1", InternalIP: "192.168.1.10", OSArch: "linux/arm64", Taints: 1},
		},
		{
			node: fakeNode("f-lost", nil, unknown, "v1.36.5+k3s1", 0),
			want: Node{Name: "f-lost", Status: "NotReady", Roles: "<none>", Version: "v1.36.5+k3s1", InternalIP: "192.168.1.10", OSArch: "linux/arm64"},
		},
		{
			node: bare,
			want: Node{Name: "g-bare", Status: "Unknown", Roles: "<none>", Version: "v1.36.5+k3s1", InternalIP: "<none>", OSArch: "<unknown>"},
		},
	}
	objs := make([]runtime.Object, 0, len(cases))
	for _, tc := range cases {
		objs = append(objs, tc.node)
	}
	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}
	got, err := c.ListNodes(context.Background())
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}
	if len(got) != len(cases) {
		t.Fatalf("got %d nodes, want %d", len(got), len(cases))
	}
	for i, tc := range cases {
		g := got[i]
		if g.Age < 48*time.Hour || g.Age > 50*time.Hour {
			t.Errorf("%s: Age = %v", tc.want.Name, g.Age)
		}
		g.Age = 0
		if g != tc.want {
			t.Errorf("node %d =\n%+v\nwant\n%+v", i, g, tc.want)
		}
	}
}

func TestListNodesPassesTheListOptionsAndWrapsErrors(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()
	var rv string
	cs.PrependReactor("list", "nodes", func(a ktesting.Action) (bool, runtime.Object, error) {
		if la, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok {
			rv = la.GetListOptions().ResourceVersion
		}
		return true, nil, errors.New("boom")
	})
	c := &Client{Clientset: cs, Context: "test"}
	_, err := c.ListNodes(context.Background(), FromCache())
	if err == nil || err.Error() != "list nodes: boom" {
		t.Errorf("err = %v, want list nodes: boom", err)
	}
	if rv != "0" {
		t.Errorf("ResourceVersion = %q, want 0", rv)
	}
}
