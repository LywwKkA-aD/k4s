package k8s

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/duration"
)

func (c *Client) describePod(ctx context.Context, namespace, name string, now time.Time) (string, error) {
	pod, err := c.Clientset.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}
	events := c.eventsFor(ctx, eventSubject{kind: "Pod", namespace: pod.Namespace, name: pod.Name, uid: podEventUID(pod)})
	return tabbed(func(w describeWriter) {
		writePodHeader(w, pod, now)
		if len(pod.Spec.InitContainers) > 0 {
			writeContainers(w, "Init Containers", "", pod, pod.Spec.InitContainers, pod.Status.InitContainerStatuses)
		}
		writeContainers(w, "Containers", "", pod, pod.Spec.Containers, pod.Status.ContainerStatuses)
		writePodConditions(w, pod.Status.Conditions)
		writeVolumes(w, "", pod.Spec.Volumes)
		w.write(0, "QoS Class:\t%s\n", pod.Status.QOSClass)
		writeLabels(w, "Node-Selectors", pod.Spec.NodeSelector)
		writeTolerations(w, "Tolerations", pod.Spec.Tolerations)
		writeEvents(w, events, now)
	}), nil
}

// podEventUID is the uid a pod's events name. A mirror pod stands for a
// static pod, whose events name the uid in its config.mirror annotation.
func podEventUID(pod *corev1.Pod) types.UID {
	if uid, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]; ok {
		return types.UID(uid)
	}
	return pod.UID
}

func writePodHeader(w describeWriter, pod *corev1.Pod, now time.Time) {
	w.write(0, "Name:\t%s\n", pod.Name)
	w.write(0, "Namespace:\t%s\n", pod.Namespace)
	if pod.Spec.Priority != nil {
		w.write(0, "Priority:\t%d\n", *pod.Spec.Priority)
	}
	if pod.Spec.PriorityClassName != "" {
		w.write(0, "Priority Class Name:\t%s\n", pod.Spec.PriorityClassName)
	}
	if pod.Spec.ServiceAccountName != "" {
		w.write(0, "Service Account:\t%s\n", pod.Spec.ServiceAccountName)
	}
	w.write(0, "Node:\t%s\n", podNode(pod))
	if pod.Status.StartTime != nil {
		w.write(0, "Start Time:\t%s\n", stamp(*pod.Status.StartTime))
	}
	writeLabels(w, "Labels", pod.Labels)
	writeAnnotations(w, "Annotations", pod.Annotations)
	writePodStatus(w, pod, now)
	w.write(0, "IP:\t%s\n", pod.Status.PodIP)
	writePodIPs(w, pod.Status.PodIPs)
	if by := controlledBy(pod.OwnerReferences); by != "" {
		w.write(0, "Controlled By:\t%s\n", by)
	}
	if pod.Status.NominatedNodeName != "" {
		w.write(0, "NominatedNodeName:\t%s\n", pod.Status.NominatedNodeName)
	}
}

// podNode is "<node>/<host IP>", or <none> before the pod is scheduled.
func podNode(pod *corev1.Pod) string {
	if pod.Spec.NodeName == "" {
		return "<none>"
	}
	return pod.Spec.NodeName + "/" + pod.Status.HostIP
}

func writePodStatus(w describeWriter, pod *corev1.Pod, now time.Time) {
	phase := pod.Status.Phase
	if pod.DeletionTimestamp != nil && phase != corev1.PodFailed && phase != corev1.PodSucceeded {
		w.write(0, "Status:\tTerminating (lasts %s)\n", since(*pod.DeletionTimestamp, now))
		if pod.DeletionGracePeriodSeconds != nil {
			w.write(0, "Termination Grace Period:\t%ds\n", *pod.DeletionGracePeriodSeconds)
		}
	} else {
		w.write(0, "Status:\t%s\n", phase)
	}
	if pod.Status.Reason != "" {
		w.write(0, "Reason:\t%s\n", pod.Status.Reason)
	}
	if pod.Status.Message != "" {
		w.write(0, "Message:\t%s\n", pod.Status.Message)
	}
}

func writePodIPs(w describeWriter, ips []corev1.PodIP) {
	if len(ips) == 0 {
		w.write(0, "IPs:\t<none>\n")
		return
	}
	w.write(0, "IPs:\n")
	for _, ip := range ips {
		w.write(1, "IP:\t%s\n", ip.IP)
	}
}

// controlledBy is "<Kind>/<name>" of the owner that controls the object,
// "" when none does.
func controlledBy(refs []metav1.OwnerReference) string {
	i := slices.IndexFunc(refs, func(r metav1.OwnerReference) bool {
		return r.Controller != nil && *r.Controller
	})
	if i < 0 {
		return ""
	}
	return refs[i].Kind + "/" + refs[i].Name
}

func writePodConditions(w describeWriter, conds []corev1.PodCondition) {
	if len(conds) == 0 {
		return
	}
	w.write(0, "Conditions:\n  Type\tStatus\n")
	for _, c := range conds {
		w.write(1, "%v \t%v \n", c.Type, c.Status)
	}
}

// writeTolerations writes the tolerations in key order, one per line, as
// kubectl does: key=value:Effect op=Exists for Ns.
func writeTolerations(w describeWriter, title string, tolerations []corev1.Toleration) {
	w.write(0, "%s:\t", title)
	if len(tolerations) == 0 {
		w.write(0, "<none>\n")
		return
	}
	sorted := slices.Clone(tolerations)
	slices.SortStableFunc(sorted, func(a, b corev1.Toleration) int { return strings.Compare(a.Key, b.Key) })
	for i, t := range sorted {
		if i > 0 {
			w.write(0, "\t")
		}
		w.write(0, "%s\n", toleration(t))
	}
}

func toleration(t corev1.Toleration) string {
	var b strings.Builder
	b.WriteString(t.Key)
	if t.Value != "" {
		b.WriteString("=" + t.Value)
	}
	if t.Effect != "" {
		b.WriteString(":" + string(t.Effect))
	}
	if t.Operator == corev1.TolerationOpExists && t.Value == "" {
		if t.Key != "" || t.Effect != "" {
			b.WriteString(" ")
		}
		b.WriteString("op=Exists")
	}
	if t.TolerationSeconds != nil {
		fmt.Fprintf(&b, " for %ds", *t.TolerationSeconds)
	}
	return b.String()
}

// since is how long ago t was, as kubectl prints an age; <unknown> for a
// zero time.
func since(t metav1.Time, now time.Time) string {
	if t.IsZero() {
		return "<unknown>"
	}
	return duration.HumanDuration(now.Sub(t.Time))
}
