package k8s

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// writeProbes writes the liveness, readiness and startup probes on one
// line each, as kubectl does.
func writeProbes(w describeWriter, c *corev1.Container) {
	for _, p := range []struct {
		title string
		probe *corev1.Probe
	}{{"Liveness", c.LivenessProbe}, {"Readiness", c.ReadinessProbe}, {"Startup", c.StartupProbe}} {
		if p.probe != nil {
			w.write(2, "%s:\t%s\n", p.title, probeText(p.probe))
		}
	}
}

func probeText(p *corev1.Probe) string {
	attrs := fmt.Sprintf("delay=%ds timeout=%ds period=%ds #success=%d #failure=%d",
		p.InitialDelaySeconds, p.TimeoutSeconds, p.PeriodSeconds, p.SuccessThreshold, p.FailureThreshold)
	switch {
	case p.Exec != nil:
		return fmt.Sprintf("exec %v %s", p.Exec.Command, attrs)
	case p.HTTPGet != nil:
		u := url.URL{Scheme: strings.ToLower(string(p.HTTPGet.Scheme)), Host: p.HTTPGet.Host, Path: p.HTTPGet.Path}
		if port := p.HTTPGet.Port.String(); port != "" {
			u.Host = net.JoinHostPort(p.HTTPGet.Host, port)
		}
		return fmt.Sprintf("http-get %s %s", u.String(), attrs)
	case p.TCPSocket != nil:
		return fmt.Sprintf("tcp-socket %s:%s %s", p.TCPSocket.Host, p.TCPSocket.Port.String(), attrs)
	case p.GRPC != nil:
		service := ""
		if p.GRPC.Service != nil {
			service = *p.GRPC.Service
		}
		return fmt.Sprintf("grpc <pod>:%d %s %s", p.GRPC.Port, service, attrs)
	}
	return "unknown " + attrs
}

// writeEnvFrom writes the config maps and secrets a container takes all
// its keys from.
func writeEnvFrom(w describeWriter, from []corev1.EnvFromSource) {
	if len(from) == 0 {
		return
	}
	w.write(2, "Environment Variables from:\n")
	for _, e := range from {
		kind, name, optional := "", "", false
		switch {
		case e.ConfigMapRef != nil:
			kind, name, optional = "ConfigMap", e.ConfigMapRef.Name, isTrue(e.ConfigMapRef.Optional)
		case e.SecretRef != nil:
			kind, name, optional = "Secret", e.SecretRef.Name, isTrue(e.SecretRef.Optional)
		}
		if e.Prefix != "" {
			kind += " with prefix '" + e.Prefix + "'"
		}
		w.write(3, "%s\t%s\tOptional: %t\n", name, kind, optional)
	}
}

// writeEnv writes a container's variables: a value as it is, a reference
// as what it points at. Secrets are never read: a secret key shows its
// name only. pod resolves field references, nil in a pod template.
func writeEnv(w describeWriter, pod *corev1.Pod, c *corev1.Container) {
	if len(c.Env) == 0 {
		w.write(2, "Environment:\t<none>\n")
		return
	}
	w.write(2, "Environment:\n")
	for _, e := range c.Env {
		if e.ValueFrom == nil {
			for i, line := range strings.Split(e.Value, "\n") {
				if i == 0 {
					w.write(3, "%s:\t%s\n", e.Name, line)
					continue
				}
				w.write(3, "\t%s\n", line)
			}
			continue
		}
		writeEnvRef(w, pod, c, e)
	}
}

func writeEnvRef(w describeWriter, pod *corev1.Pod, c *corev1.Container, e corev1.EnvVar) {
	from := e.ValueFrom
	switch {
	case from.FieldRef != nil:
		value := ""
		if pod != nil {
			value = fieldValue(pod, from.FieldRef.FieldPath)
		}
		w.write(3, "%s:\t%s (%s:%s)\n", e.Name, value, from.FieldRef.APIVersion, from.FieldRef.FieldPath)
	case from.ResourceFieldRef != nil:
		w.write(3, "%s:\t%s (%s)\n", e.Name, resourceValue(c, from.ResourceFieldRef), from.ResourceFieldRef.Resource)
	case from.SecretKeyRef != nil:
		ref := from.SecretKeyRef
		w.write(3, "%s:\t<set to the key '%s' in secret '%s'>\tOptional: %t\n", e.Name, ref.Key, ref.Name, isTrue(ref.Optional))
	case from.ConfigMapKeyRef != nil:
		ref := from.ConfigMapKeyRef
		w.write(3, "%s:\t<set to the key '%s' of config map '%s'>\tOptional: %t\n", e.Name, ref.Key, ref.Name, isTrue(ref.Optional))
	}
}

// fieldValue reads the downward-API field path of pod that an env var
// points at, "" for a path it does not know.
func fieldValue(pod *corev1.Pod, path string) string {
	if key, ok := mapKey(path, "metadata.labels"); ok {
		return pod.Labels[key]
	}
	if key, ok := mapKey(path, "metadata.annotations"); ok {
		return pod.Annotations[key]
	}
	switch path {
	case "metadata.name":
		return pod.Name
	case "metadata.namespace":
		return pod.Namespace
	case "metadata.uid":
		return string(pod.UID)
	case "spec.nodeName":
		return pod.Spec.NodeName
	case "spec.serviceAccountName":
		return pod.Spec.ServiceAccountName
	case "status.hostIP":
		return pod.Status.HostIP
	case "status.podIP":
		return pod.Status.PodIP
	case "status.podIPs":
		ips := make([]string, 0, len(pod.Status.PodIPs))
		for _, ip := range pod.Status.PodIPs {
			ips = append(ips, ip.IP)
		}
		return strings.Join(ips, ",")
	}
	return ""
}

// mapKey reads "<prefix>['key']".
func mapKey(path, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(path, prefix+"['")
	if !ok {
		return "", false
	}
	return strings.CutSuffix(rest, "']")
}

// resourceValue is the value of a resource field reference as the
// container would see it: the quantity over the divisor, rounded up. An
// unset limit is the node's allocatable, as kubectl says.
func resourceValue(c *corev1.Container, sel *corev1.ResourceFieldSelector) string {
	limit, name, ok := strings.Cut(sel.Resource, ".")
	if !ok {
		return ""
	}
	list := c.Resources.Requests
	if limit == "limits" {
		list = c.Resources.Limits
	}
	q := list[corev1.ResourceName(name)]
	divisor := sel.Divisor
	if divisor.IsZero() {
		divisor = resource.MustParse("1")
	}
	var v int64
	switch corev1.ResourceName(name) {
	case corev1.ResourceCPU:
		v = int64(math.Ceil(float64(q.MilliValue()) / float64(divisor.MilliValue())))
	case corev1.ResourceMemory, corev1.ResourceEphemeralStorage:
		v = int64(math.Ceil(float64(q.Value()) / float64(divisor.Value())))
	default:
		return ""
	}
	if v == 0 && limit == "limits" {
		return "node allocatable"
	}
	return strconv.FormatInt(v, 10)
}
