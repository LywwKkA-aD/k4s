package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Service is a presentation-friendly snapshot of a core/v1 Service.
type Service struct {
	Name       string
	Namespace  string
	Type       string // ClusterIP / NodePort / LoadBalancer / ExternalName
	ClusterIP  string
	ExternalIP string // pretty-printed: "<none>" / "<pending>" / "1.2.3.4,host.example"
	Ports      string // "80/TCP" or "80:31000/TCP" for NodePort
	Age        time.Duration
}

// ListServices returns services in the given namespace, or all namespaces when
// namespace == "".
func (c *Client) ListServices(ctx context.Context, namespace string) ([]Service, error) {
	list, err := c.Clientset.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(list.Items))
	now := time.Now()
	for i := range list.Items {
		s := &list.Items[i]
		out = append(out, Service{
			Name:       s.Name,
			Namespace:  s.Namespace,
			Type:       string(s.Spec.Type),
			ClusterIP:  serviceClusterIP(s),
			ExternalIP: serviceExternalIP(s),
			Ports:      servicePorts(s),
			Age:        now.Sub(s.CreationTimestamp.Time),
		})
	}
	return out, nil
}

// serviceClusterIP returns "<none>" for headless services so the table column
// reads consistently with `kubectl get svc`.
func serviceClusterIP(s *corev1.Service) string {
	if s.Spec.ClusterIP == "" || s.Spec.ClusterIP == corev1.ClusterIPNone {
		return "<none>"
	}
	return s.Spec.ClusterIP
}

// serviceExternalIP renders the external-IP column using kubectl's rules:
//
//	ClusterIP    → "<none>"
//	NodePort     → "<none>"  (NodePort exposes via node IPs, not an external svc IP)
//	LoadBalancer → comma-joined ingress IPs / hostnames, or "<pending>"
//	ExternalName → the external DNS name
func serviceExternalIP(s *corev1.Service) string {
	switch s.Spec.Type {
	case corev1.ServiceTypeLoadBalancer:
		if len(s.Status.LoadBalancer.Ingress) == 0 {
			return "<pending>"
		}
		ips := make([]string, 0, len(s.Status.LoadBalancer.Ingress))
		for _, ing := range s.Status.LoadBalancer.Ingress {
			switch {
			case ing.IP != "":
				ips = append(ips, ing.IP)
			case ing.Hostname != "":
				ips = append(ips, ing.Hostname)
			}
		}
		if len(ips) == 0 {
			return "<pending>"
		}
		return strings.Join(ips, ",")
	case corev1.ServiceTypeExternalName:
		if s.Spec.ExternalName != "" {
			return s.Spec.ExternalName
		}
		return "<none>"
	default:
		return "<none>"
	}
}

// servicePorts mirrors `kubectl get svc`'s PORT(S) column. NodePorts are
// rendered as "port:nodePort/proto" so users can see both at a glance.
func servicePorts(s *corev1.Service) string {
	if len(s.Spec.Ports) == 0 {
		return "<none>"
	}
	parts := make([]string, 0, len(s.Spec.Ports))
	nodePort := s.Spec.Type == corev1.ServiceTypeNodePort || s.Spec.Type == corev1.ServiceTypeLoadBalancer
	for _, p := range s.Spec.Ports {
		proto := p.Protocol
		if proto == "" {
			proto = corev1.ProtocolTCP
		}
		if nodePort && p.NodePort > 0 {
			parts = append(parts, fmt.Sprintf("%d:%d/%s", p.Port, p.NodePort, proto))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.Port, proto))
		}
	}
	return strings.Join(parts, ",")
}
