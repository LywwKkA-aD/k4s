package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Client) describeDeployment(ctx context.Context, namespace, name string, now time.Time) (string, error) {
	d, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get deployment: %w", err)
	}
	events := c.eventsFor(ctx, eventSubject{kind: "Deployment", namespace: d.Namespace, name: d.Name, uid: d.UID})
	return tabbed(func(w describeWriter) {
		writeDeployment(w, d)
		writePodTemplate(w, &d.Spec.Template)
		writeDeploymentConditions(w, d.Status.Conditions)
		writeEvents(w, events, now)
	}), nil
}

func writeDeployment(w describeWriter, d *appsv1.Deployment) {
	w.write(0, "Name:\t%s\n", d.Name)
	w.write(0, "Namespace:\t%s\n", d.Namespace)
	w.write(0, "CreationTimestamp:\t%s\n", stamp(d.CreationTimestamp))
	writeLabels(w, "Labels", d.Labels)
	writeAnnotations(w, "Annotations", d.Annotations)
	w.write(0, "Selector:\t%s\n", selectorText(d.Spec.Selector))
	desired := int32(1)
	if d.Spec.Replicas != nil {
		desired = *d.Spec.Replicas
	}
	st := d.Status
	w.write(0, "Replicas:\t%d desired | %d updated | %d total | %d available | %d unavailable\n",
		desired, st.UpdatedReplicas, st.Replicas, st.AvailableReplicas, st.UnavailableReplicas)
	w.write(0, "StrategyType:\t%s\n", d.Spec.Strategy.Type)
	w.write(0, "MinReadySeconds:\t%d\n", d.Spec.MinReadySeconds)
	if ru := d.Spec.Strategy.RollingUpdate; ru != nil {
		w.write(0, "RollingUpdateStrategy:\t%s max unavailable, %s max surge\n", intOrPercent(ru.MaxUnavailable), intOrPercent(ru.MaxSurge))
	}
}

func selectorText(sel *metav1.LabelSelector) string {
	s, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil {
		return "<invalid>"
	}
	return s.String()
}

func intOrPercent(v *intstr.IntOrString) string {
	if v == nil {
		return "<nil>"
	}
	return v.String()
}

// writePodTemplate writes a "Pod Template:" section: a pod without status,
// one level in.
func writePodTemplate(w describeWriter, t *corev1.PodTemplateSpec) {
	w.write(0, "Pod Template:\n")
	writeLabels(w, "  Labels", t.Labels)
	if len(t.Annotations) > 0 {
		writeAnnotations(w, "  Annotations", t.Annotations)
	}
	if t.Spec.ServiceAccountName != "" {
		w.write(1, "Service Account:\t%s\n", t.Spec.ServiceAccountName)
	}
	if len(t.Spec.InitContainers) > 0 {
		writeContainers(w, "Init Containers", "  ", t.Spec.InitContainers, nil)
	}
	writeContainers(w, "Containers", "  ", t.Spec.Containers, nil)
	writeVolumes(w, "  ", t.Spec.Volumes)
	writeLabels(w, "  Node-Selectors", t.Spec.NodeSelector)
	writeTolerations(w, "  Tolerations", t.Spec.Tolerations)
	if t.Spec.PriorityClassName != "" {
		w.write(1, "Priority Class Name:\t%s\n", t.Spec.PriorityClassName)
	}
}

func writeDeploymentConditions(w describeWriter, conds []appsv1.DeploymentCondition) {
	if len(conds) == 0 {
		return
	}
	w.write(0, "Conditions:\n  Type\tStatus\tReason\n")
	w.write(1, "----\t------\t------\n")
	for _, c := range conds {
		w.write(1, "%s \t%s\t%s\n", c.Type, c.Status, c.Reason)
	}
}

func (c *Client) describeService(ctx context.Context, namespace, name string, now time.Time) (string, error) {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get service: %w", err)
	}
	events := c.eventsFor(ctx, eventSubject{kind: "Service", namespace: svc.Namespace, name: svc.Name, uid: svc.UID})
	return tabbed(func(w describeWriter) {
		writeServiceHeader(w, svc)
		for i := range svc.Spec.Ports {
			writeServicePort(w, &svc.Spec.Ports[i])
		}
		writeServicePolicies(w, &svc.Spec)
		writeEvents(w, events, now)
	}), nil
}

func writeServiceHeader(w describeWriter, svc *corev1.Service) {
	spec := &svc.Spec
	w.write(0, "Name:\t%s\n", svc.Name)
	w.write(0, "Namespace:\t%s\n", svc.Namespace)
	writeLabels(w, "Labels", svc.Labels)
	writeAnnotations(w, "Annotations", svc.Annotations)
	w.write(0, "Selector:\t%s\n", labels.FormatLabels(spec.Selector))
	w.write(0, "Type:\t%s\n", spec.Type)
	if spec.IPFamilyPolicy != nil {
		w.write(0, "IP Family Policy:\t%s\n", *spec.IPFamilyPolicy)
	}
	families := make([]string, 0, len(spec.IPFamilies))
	for _, f := range spec.IPFamilies {
		families = append(families, string(f))
	}
	w.write(0, "IP Families:\t%s\n", orNone(strings.Join(families, ",")))
	w.write(0, "IP:\t%s\n", spec.ClusterIP)
	w.write(0, "IPs:\t%s\n", orNone(strings.Join(spec.ClusterIPs, ",")))
	if len(spec.ExternalIPs) > 0 {
		w.write(0, "External IPs:\t%s\n", strings.Join(spec.ExternalIPs, ","))
	}
	if spec.ExternalName != "" {
		w.write(0, "External Name:\t%s\n", spec.ExternalName)
	}
	if ingress := svc.Status.LoadBalancer.Ingress; len(ingress) > 0 {
		w.write(0, "LoadBalancer Ingress:\t%s\n", ingressText(ingress))
	}
}

func ingressText(ingress []corev1.LoadBalancerIngress) string {
	out := make([]string, 0, len(ingress))
	for _, in := range ingress {
		switch {
		case in.IP != "":
			out = append(out, in.IP)
		case in.Hostname != "":
			out = append(out, in.Hostname)
		}
	}
	return strings.Join(out, ", ")
}

func writeServicePort(w describeWriter, p *corev1.ServicePort) {
	name := p.Name
	if name == "" {
		name = "<unset>"
	}
	proto := p.Protocol
	if proto == "" {
		proto = corev1.ProtocolTCP
	}
	w.write(0, "Port:\t%s\t%d/%s\n", name, p.Port, proto)
	if p.TargetPort.Type == intstr.Int {
		w.write(0, "TargetPort:\t%d/%s\n", p.TargetPort.IntVal, proto)
	} else {
		w.write(0, "TargetPort:\t%s/%s\n", p.TargetPort.StrVal, proto)
	}
	if p.NodePort != 0 {
		w.write(0, "NodePort:\t%s\t%d/%s\n", name, p.NodePort, proto)
	}
}

func writeServicePolicies(w describeWriter, spec *corev1.ServiceSpec) {
	w.write(0, "Session Affinity:\t%s\n", spec.SessionAffinity)
	if spec.ExternalTrafficPolicy != "" {
		w.write(0, "External Traffic Policy:\t%s\n", spec.ExternalTrafficPolicy)
	}
	if spec.InternalTrafficPolicy != nil {
		w.write(0, "Internal Traffic Policy:\t%s\n", *spec.InternalTrafficPolicy)
	}
	if spec.HealthCheckNodePort != 0 {
		w.write(0, "HealthCheck NodePort:\t%d\n", spec.HealthCheckNodePort)
	}
	if len(spec.LoadBalancerSourceRanges) > 0 {
		w.write(0, "LoadBalancer Source Ranges:\t%s\n", strings.Join(spec.LoadBalancerSourceRanges, ","))
	}
}
