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

// kubectl writes the entire pod spec back as this annotation on every apply,
// which is huge, multiline JSON. Hide it the same way `kubectl describe` would
// in spirit — users that actually want it can read the raw object.
const lastAppliedConfigKey = "kubectl.kubernetes.io/last-applied-configuration"

// maxAnnotationValueLen caps annotation values rendered in the TUI. Anything
// longer is replaced with the prefix + an ellipsis, since the TUI is the wrong
// surface for inspecting multi-KB JSON blobs.
const maxAnnotationValueLen = 200

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

	writeStringMap(&sb, "Labels", pod.Labels, nil, 0)
	writeStringMap(&sb, "Annotations", pod.Annotations, []string{lastAppliedConfigKey}, maxAnnotationValueLen)

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

	// Events are best-effort — a permission failure on the events resource
	// should not break the whole describe view.
	writeEvents(ctx, c, &sb, namespace, "Pod", name)

	return sb.String(), nil
}

// DescribeDeployment is the kubectl-describe equivalent for a Deployment.
// Reuses the helpers (writeStringMap, writeContainer, writeEvents) that the
// pod-describe path defines, so the visual format stays consistent.
func (c *Client) DescribeDeployment(ctx context.Context, namespace, name string) (string, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get deployment: %w", err)
	}

	var sb strings.Builder
	field := func(label, value string) { fmt.Fprintf(&sb, "%-13s %s\n", label, value) }

	field("Name:", dep.Name)
	field("Namespace:", dep.Namespace)
	if !dep.CreationTimestamp.IsZero() {
		field("Age:", HumanizeDuration(time.Since(dep.CreationTimestamp.Time)))
		field("Created:", dep.CreationTimestamp.Format(time.RFC3339))
	}
	if dep.Spec.Selector != nil {
		sel, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
		if err == nil {
			field("Selector:", sel.String())
		}
	}
	field("Strategy:", string(dep.Spec.Strategy.Type))

	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	field("Replicas:", fmt.Sprintf(
		"%d desired · %d current · %d ready · %d updated · %d available",
		desired,
		dep.Status.Replicas,
		dep.Status.ReadyReplicas,
		dep.Status.UpdatedReplicas,
		dep.Status.AvailableReplicas,
	))

	writeStringMap(&sb, "Labels", dep.Labels, nil, 0)
	writeStringMap(&sb, "Annotations", dep.Annotations, []string{lastAppliedConfigKey}, maxAnnotationValueLen)

	sb.WriteString("\nPod Template:\n")
	if labels := dep.Spec.Template.Labels; len(labels) > 0 {
		fmt.Fprintf(&sb, "  Labels: %s\n", inlineLabels(labels))
	}
	sb.WriteString("  Containers:\n")
	for i := range dep.Spec.Template.Spec.Containers {
		writeContainer(&sb, &dep.Spec.Template.Spec.Containers[i], nil)
	}

	if len(dep.Status.Conditions) > 0 {
		sb.WriteString("\nConditions:\n")
		for _, cnd := range dep.Status.Conditions {
			reason := cnd.Reason
			if reason == "" {
				reason = "—"
			}
			fmt.Fprintf(&sb, "  %-20s %s · %s\n", cnd.Type, cnd.Status, reason)
		}
	}

	writeEvents(ctx, c, &sb, namespace, "Deployment", name)

	return sb.String(), nil
}

// inlineLabels formats a label map as "k1=v1, k2=v2" with sorted keys, for
// places where a single-line summary is more useful than the indented
// "Labels:\n  k=v" block writeStringMap produces.
func inlineLabels(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+m[k])
	}
	return strings.Join(pairs, ", ")
}

// writeStringMap renders a string map as a "Label:\n  k=v" block.
//   - skipKeys are filtered out before rendering (e.g. last-applied-configuration).
//   - maxValueLen >0 truncates long values; 0 means render in full.
//   - newlines inside values are replaced with spaces so a single annotation
//     never spans multiple lines (which broke the layout in the previous run).
func writeStringMap(w *strings.Builder, label string, m map[string]string, skipKeys []string, maxValueLen int) {
	skip := make(map[string]bool, len(skipKeys))
	for _, k := range skipKeys {
		skip[k] = true
	}

	// Filter first so the "<none>" branch fires when *visible* keys are empty.
	visible := make(map[string]string, len(m))
	for k, v := range m {
		if skip[k] {
			continue
		}
		visible[k] = v
	}

	if len(visible) == 0 {
		fmt.Fprintf(w, "%-13s <none>\n", label+":")
		return
	}
	keys := make([]string, 0, len(visible))
	for k := range visible {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "%s:\n", label)
	for _, k := range keys {
		v := strings.ReplaceAll(visible[k], "\n", " ")
		if maxValueLen > 0 {
			v = truncateString(v, maxValueLen)
		}
		fmt.Fprintf(w, "  %s=%s\n", k, v)
	}
}

func truncateString(s string, maxLen int) string {
	if maxLen <= 1 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
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

// writeEvents queries Events in the namespace and renders the ones whose
// involvedObject matches (kind, name), most recent first. Filtering is done
// client-side because the fake clientset does not honour FieldSelector and
// we want the production and test code paths to behave identically.
func writeEvents(ctx context.Context, c *Client, w *strings.Builder, namespace, kind, name string) {
	list, err := c.Clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}
	mine := make([]corev1.Event, 0)
	for _, e := range list.Items {
		if e.InvolvedObject.Kind == kind && e.InvolvedObject.Name == name {
			mine = append(mine, e)
		}
	}
	if len(mine) == 0 {
		return
	}
	sort.Slice(mine, func(i, j int) bool {
		return eventTime(mine[i]).After(eventTime(mine[j]))
	})

	w.WriteString("\nEvents:\n")
	fmt.Fprintf(w, "  %-8s %-22s %-8s %s\n", "TYPE", "REASON", "AGE", "MESSAGE")
	for _, e := range mine {
		age := "?"
		if t := eventTime(e); !t.IsZero() {
			age = HumanizeDuration(time.Since(t))
		}
		msg := strings.ReplaceAll(e.Message, "\n", " ")
		fmt.Fprintf(w, "  %-8s %-22s %-8s %s\n", e.Type, e.Reason, age, msg)
	}
}

// eventTime picks the most informative timestamp Kubernetes recorded for the
// event. New-style events use EventTime; classic events use LastTimestamp.
func eventTime(e corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.FirstTimestamp.Time
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
