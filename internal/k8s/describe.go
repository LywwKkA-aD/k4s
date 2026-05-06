package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DescribePod returns a human-readable description of the given pod, modelled
// after `kubectl describe pod` but trimmed to the fields k4s users care about.
func (c *Client) DescribePod(ctx context.Context, namespace, name string) (string, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}

	var sb strings.Builder
	field := func(label, value string) { fmt.Fprintf(&sb, "%-13s %s\n", label, value) }

	field("Name:", pod.Name)
	field("Namespace:", pod.Namespace)
	if pod.Spec.NodeName != "" {
		field("Node:", pod.Spec.NodeName)
	}
	if !pod.CreationTimestamp.IsZero() {
		field("Age:", HumanizeDuration(time.Since(pod.CreationTimestamp.Time)))
		field("Started:", pod.CreationTimestamp.Format(time.RFC3339))
	}
	field("Status:", podStatus(pod))
	if pod.Status.PodIP != "" {
		field("Pod IP:", pod.Status.PodIP)
	}
	if pod.Status.QOSClass != "" {
		field("QoS Class:", string(pod.Status.QOSClass))
	}

	writeStringMap(&sb, "Labels", pod.Labels)
	writeStringMap(&sb, "Annotations", pod.Annotations)

	sb.WriteString("\nContainers:\n")
	for i := range pod.Spec.Containers {
		writeContainer(&sb, &pod.Spec.Containers[i], findContainerStatus(pod, pod.Spec.Containers[i].Name))
	}

	if len(pod.Status.Conditions) > 0 {
		sb.WriteString("\nConditions:\n")
		for _, c := range pod.Status.Conditions {
			fmt.Fprintf(&sb, "  %-18s %s\n", c.Type, c.Status)
		}
	}

	if len(pod.Spec.Volumes) > 0 {
		sb.WriteString("\nVolumes:\n")
		for _, v := range pod.Spec.Volumes {
			writeVolume(&sb, v)
		}
	}

	return sb.String(), nil
}

func writeStringMap(w *strings.Builder, label string, m map[string]string) {
	if len(m) == 0 {
		fmt.Fprintf(w, "%-13s <none>\n", label+":")
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "%s:\n", label)
	for _, k := range keys {
		fmt.Fprintf(w, "  %s=%s\n", k, m[k])
	}
}

func writeContainer(w *strings.Builder, c *corev1.Container, status *corev1.ContainerStatus) {
	fmt.Fprintf(w, "  %s:\n", c.Name)
	fmt.Fprintf(w, "    Image:          %s\n", c.Image)
	if len(c.Ports) > 0 {
		ports := make([]string, 0, len(c.Ports))
		for _, p := range c.Ports {
			proto := p.Protocol
			if proto == "" {
				proto = corev1.ProtocolTCP
			}
			ports = append(ports, fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		}
		fmt.Fprintf(w, "    Ports:          %s\n", strings.Join(ports, ", "))
	}
	if status != nil {
		fmt.Fprintf(w, "    State:          %s\n", containerStateString(status.State))
		fmt.Fprintf(w, "    Ready:          %t\n", status.Ready)
		fmt.Fprintf(w, "    Restart Count:  %d\n", status.RestartCount)
	}
	if len(c.Resources.Limits) > 0 || len(c.Resources.Requests) > 0 {
		fmt.Fprintln(w, "    Resources:")
		for k, v := range c.Resources.Requests {
			fmt.Fprintf(w, "      requests.%s: %s\n", k, v.String())
		}
		for k, v := range c.Resources.Limits {
			fmt.Fprintf(w, "      limits.%s:   %s\n", k, v.String())
		}
	}
	if len(c.VolumeMounts) > 0 {
		fmt.Fprintln(w, "    Mounts:")
		for _, vm := range c.VolumeMounts {
			ro := ""
			if vm.ReadOnly {
				ro = " (ro)"
			}
			fmt.Fprintf(w, "      %s from %s%s\n", vm.MountPath, vm.Name, ro)
		}
	}
}

func writeVolume(w *strings.Builder, v corev1.Volume) {
	fmt.Fprintf(w, "  %s:\n", v.Name)
	switch {
	case v.ConfigMap != nil:
		fmt.Fprintln(w, "    type:  configMap")
		fmt.Fprintf(w, "    name:  %s\n", v.ConfigMap.Name)
	case v.Secret != nil:
		fmt.Fprintln(w, "    type:  secret")
		fmt.Fprintf(w, "    name:  %s\n", v.Secret.SecretName)
	case v.PersistentVolumeClaim != nil:
		fmt.Fprintln(w, "    type:   persistentVolumeClaim")
		fmt.Fprintf(w, "    claim:  %s\n", v.PersistentVolumeClaim.ClaimName)
	case v.EmptyDir != nil:
		fmt.Fprintln(w, "    type:  emptyDir")
	default:
		fmt.Fprintln(w, "    type:  (other)")
	}
}

func containerStateString(s corev1.ContainerState) string {
	switch {
	case s.Running != nil:
		return fmt.Sprintf("Running (since %s)", s.Running.StartedAt.Format(time.RFC3339))
	case s.Waiting != nil:
		if s.Waiting.Reason != "" {
			return fmt.Sprintf("Waiting (%s)", s.Waiting.Reason)
		}
		return "Waiting"
	case s.Terminated != nil:
		if s.Terminated.Reason != "" {
			return fmt.Sprintf("Terminated (%s, exit %d)", s.Terminated.Reason, s.Terminated.ExitCode)
		}
		return fmt.Sprintf("Terminated (exit %d)", s.Terminated.ExitCode)
	}
	return "(unknown)"
}

func findContainerStatus(p *corev1.Pod, name string) *corev1.ContainerStatus {
	for i := range p.Status.ContainerStatuses {
		if p.Status.ContainerStatuses[i].Name == name {
			return &p.Status.ContainerStatuses[i]
		}
	}
	return nil
}
