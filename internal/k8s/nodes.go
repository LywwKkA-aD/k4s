package k8s

import (
	"context"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
)

const (
	// nodeRolePrefix labels a node with a role: node-role.kubernetes.io/<role>.
	nodeRolePrefix = "node-role.kubernetes.io/"
	// nodeRoleLabel is the older label that names one role as its value.
	nodeRoleLabel = "kubernetes.io/role"
)

// Node is a node as `kubectl get nodes -o wide` lists it, plus its OS and
// architecture together and its taints counted.
type Node struct {
	Name string
	// Status is kubectl's STATUS: Ready, NotReady or Unknown, with
	// SchedulingDisabled after a comma on a cordoned node.
	Status string
	// Roles are the node's roles, sorted and comma-joined as kubectl
	// joins them: k3s servers show control-plane,master, or
	// control-plane,etcd,master with embedded etcd. "<none>" for an agent.
	Roles      string
	Age        time.Duration
	Version    string
	InternalIP string // "<none>" when the node reports none
	OSArch     string // "linux/arm64"
	Taints     int
}

// ListNodes lists the cluster's nodes, a consistent read unless opts say
// otherwise.
func (c *Client) ListNodes(ctx context.Context, opts ...ListOption) ([]Node, error) {
	list, err := c.Clientset.CoreV1().Nodes().List(ctx, listOptions(opts))
	if err != nil {
		return nil, &CallError{Verb: "list", Resource: "nodes", ClusterScoped: true, Err: err}
	}
	now := time.Now()
	out := make([]Node, 0, len(list.Items))
	for i := range list.Items {
		n := &list.Items[i]
		out = append(out, Node{
			Name:       n.Name,
			Status:     nodeStatus(n),
			Roles:      nodeRoles(n),
			Age:        now.Sub(n.CreationTimestamp.Time),
			Version:    n.Status.NodeInfo.KubeletVersion,
			InternalIP: nodeInternalIP(n),
			OSArch:     nodeOSArch(n),
			Taints:     len(n.Spec.Taints),
		})
	}
	return out, nil
}

// nodeStatus is kubectl's STATUS: the Ready condition read as Ready or
// NotReady, Unknown without one, and SchedulingDisabled when cordoned.
func nodeStatus(n *corev1.Node) string {
	status := "Unknown"
	for _, c := range n.Status.Conditions {
		if c.Type != corev1.NodeReady {
			continue
		}
		status = "NotReady"
		if c.Status == corev1.ConditionTrue {
			status = "Ready"
		}
	}
	if n.Spec.Unschedulable {
		status += ",SchedulingDisabled"
	}
	return status
}

// nodeRoles is kubectl's ROLES: every node-role.kubernetes.io/<role>
// label and the value of kubernetes.io/role, sorted without repeats.
func nodeRoles(n *corev1.Node) string {
	var roles []string
	for k, v := range n.Labels {
		switch {
		case strings.HasPrefix(k, nodeRolePrefix):
			if role := strings.TrimPrefix(k, nodeRolePrefix); role != "" {
				roles = append(roles, role)
			}
		case k == nodeRoleLabel && v != "":
			roles = append(roles, v)
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	slices.Sort(roles)
	return strings.Join(slices.Compact(roles), ",")
}

func nodeInternalIP(n *corev1.Node) string {
	for _, a := range n.Status.Addresses {
		if a.Type == corev1.NodeInternalIP {
			return a.Address
		}
	}
	return "<none>"
}

func nodeOSArch(n *corev1.Node) string {
	info := n.Status.NodeInfo
	if info.OperatingSystem == "" && info.Architecture == "" {
		return "<unknown>"
	}
	return info.OperatingSystem + "/" + info.Architecture
}
