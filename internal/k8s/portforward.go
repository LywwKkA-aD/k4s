package k8s

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// tryListen abstracts the listener creation so port-availability checks
// can be exercised in tests without poking at real OS ports.
var tryListen = func(port uint16) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// CheckLocalPortAvailable tries to bind 127.0.0.1:port and immediately
// closes the listener. Returns nil if we can claim the port, or the OS
// error otherwise — typically "bind: address already in use" or "bind:
// permission denied" for ports < 1024.
//
// Pure best-effort: the port might get grabbed between this check and
// the actual forward starting (TOCTOU). The point is to give the user
// a fast, clear error in the common case instead of a wall of stderr
// from kubectl.
func CheckLocalPortAvailable(port uint16) error {
	ln, err := tryListen(port)
	if err != nil {
		return err
	}
	return ln.Close()
}

// ResolveServiceToPod picks the first ready pod behind the Service and
// translates svcPort to the container port on that pod, the way
// `kubectl port-forward svc/X LOCAL:REMOTE` does: REMOTE is a Service port,
// and its targetPort (a number, or a name looked up in the pod's
// containers) is where the tunnel lands. svcPort 0 selects the first
// declared port.
func (c *Client) ResolveServiceToPod(ctx context.Context, namespace, serviceName string, svcPort int32) (*corev1.Pod, int32, error) {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return nil, 0, fmt.Errorf("get service: %w", err)
	}
	if len(svc.Spec.Selector) == 0 {
		// Headless or selector-less services have no backing pods we
		// can resolve. The user has to forward to a pod directly.
		return nil, 0, fmt.Errorf("service %q has no selector; forward to a pod instead", serviceName)
	}

	port, err := pickServicePort(svc, svcPort)
	if err != nil {
		return nil, 0, err
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector).String()
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, 0, fmt.Errorf("list pods: %w", err)
	}
	pod := firstReadyPod(pods.Items)
	if pod == nil {
		return nil, 0, fmt.Errorf("no ready pod backs service %q", serviceName)
	}

	target, err := resolveTargetPort(port, pod)
	if err != nil {
		return nil, 0, err
	}
	return pod, target, nil
}

// ResolveDeploymentToPod returns the first ready pod produced by the
// deployment. We resolve via the deployment's pod-template label
// selector — same approach kubectl uses.
func (c *Client) ResolveDeploymentToPod(ctx context.Context, namespace, deployName string) (string, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get deployment: %w", err)
	}
	if dep.Spec.Selector == nil {
		return "", fmt.Errorf("deployment %q has no pod selector", deployName)
	}
	// LabelSelectorAsSelector honours matchExpressions too — a plain
	// SelectorFromSet over MatchLabels would silently drop deployments whose
	// selector is expression-only (e.g. `app In (demo)`), which the API
	// considers perfectly valid.
	selector, err := metav1.LabelSelectorAsSelector(dep.Spec.Selector)
	if err != nil {
		return "", fmt.Errorf("convert selector: %w", err)
	}
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}
	pod := firstReadyPod(pods.Items)
	if pod == nil {
		return "", fmt.Errorf("no ready pod for deployment %q", deployName)
	}
	return pod.Name, nil
}

// pickServicePort returns the ServicePort whose Port equals port, or the
// first declared one when port is 0. A missing port is an error that lists
// what the Service does expose, so a mistyped REMOTE is easy to fix.
func pickServicePort(svc *corev1.Service, port int32) (corev1.ServicePort, error) {
	if len(svc.Spec.Ports) == 0 {
		return corev1.ServicePort{}, fmt.Errorf("service %q exposes no ports", svc.Name)
	}
	if port == 0 {
		return svc.Spec.Ports[0], nil
	}
	declared := make([]string, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		if p.Port == port {
			return p, nil
		}
		declared = append(declared, strconv.Itoa(int(p.Port)))
	}
	return corev1.ServicePort{}, fmt.Errorf("service %q has no port %d (ports: %s)",
		svc.Name, port, strings.Join(declared, ", "))
}

// resolveTargetPort maps a ServicePort to the container port on pod: a
// numeric targetPort as is, a named one through the pod's container ports,
// and an omitted one to the Service port itself (what the API defaults it to).
func resolveTargetPort(port corev1.ServicePort, pod *corev1.Pod) (int32, error) {
	target := port.TargetPort
	if target.Type == intstr.String && target.StrVal != "" {
		return containerPortByName(pod, target.StrVal)
	}
	if target.Type == intstr.Int && target.IntVal > 0 {
		return target.IntVal, nil
	}
	return port.Port, nil
}

// containerPortByName finds the named port across the pod's containers.
// Like kubectl, it does not search init containers.
func containerPortByName(pod *corev1.Pod, name string) (int32, error) {
	for _, c := range pod.Spec.Containers {
		for _, p := range c.Ports {
			if p.Name == name {
				return p.ContainerPort, nil
			}
		}
	}
	return 0, fmt.Errorf("pod %q has no container port named %q", pod.Name, name)
}

// firstReadyPod walks pods in list order and returns the first that is
// Running + Ready, or nil, so forwards land on a deterministic replica.
func firstReadyPod(items []corev1.Pod) *corev1.Pod {
	for i := range items {
		if isRunningAndReady(&items[i]) {
			return &items[i]
		}
	}
	return nil
}

func isRunningAndReady(p *corev1.Pod) bool {
	if p.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}
