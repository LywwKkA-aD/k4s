package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardSession is the live handle for one in-process forward.
//
// Lifecycle:
//  1. Caller invokes StartPodPortForward — it returns immediately and a
//     goroutine drives the SPDY tunnel.
//  2. The goroutine signals Ready (closed channel) once the local socket
//     is accepting connections, or Err (single error) if it failed.
//  3. Close the StopCh to terminate. Done closes when the goroutine has
//     fully unwound, so the caller can `<-sess.Done` to block on
//     teardown.
type PortForwardSession struct {
	StopCh chan struct{}
	Ready  chan struct{}
	Err    chan error
	Done   chan struct{}
	// Pod is the resolved pod the forward ended up targeting — useful
	// when the caller asked for a service/deployment and we want to
	// surface which replica actually serves the connection.
	Pod string
}

// Close stops the forward if it hasn't already. Idempotent — safe to call
// after the session has died on its own.
func (s *PortForwardSession) Close() {
	select {
	case <-s.StopCh:
		// already closed
	default:
		close(s.StopCh)
	}
}

// StartPodPortForward opens a local TCP listener on localPort and tunnels
// traffic to remotePort inside the named pod via SPDY. Returns a session
// the caller controls; do not call StartPodPortForward.ForwardPorts —
// that is what the background goroutine does.
//
// The forward dies when StopCh is closed *or* when the k4s process exits.
// State persistence lives a layer above (forwards.Manager).
func (c *Client) StartPodPortForward(ctx context.Context, namespace, podName string, localPort, remotePort uint16) (*PortForwardSession, error) {
	if c.RestConfig == nil {
		return nil, fmt.Errorf("client built without rest config; port-forward is unavailable")
	}

	transport, upgrader, err := spdy.RoundTripperFor(c.RestConfig)
	if err != nil {
		return nil, fmt.Errorf("build SPDY transport: %w", err)
	}

	host := c.RestConfig.Host
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	serverURL := &url.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName),
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, serverURL)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	// stderr goes to os.Stderr instead of being silently swallowed —
	// kubectl messages like "lost connection to pod" help users debug
	// when a forward goes unhealthy.
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, os.Stdout, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("build forwarder: %w", err)
	}

	go func() {
		defer close(doneCh)
		// ForwardPorts blocks until stopCh closes or the connection
		// dies. Either way we surface the result on Err (non-blocking
		// because Err is buffered size 1) so the supervisor can record
		// the cause.
		if err := fw.ForwardPorts(); err != nil {
			select {
			case errCh <- err:
			default:
			}
		}
	}()

	// Respect caller context: if it cancels before Ready fires, tear
	// the forward down. Runs in its own goroutine so StartPodPortForward
	// returns immediately.
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-stopCh:
			default:
				close(stopCh)
			}
		case <-doneCh:
		}
	}()

	return &PortForwardSession{
		StopCh: stopCh,
		Ready:  readyCh,
		Err:    errCh,
		Done:   doneCh,
		Pod:    podName,
	}, nil
}

// ResolveServiceToPod walks Service → label selector → first ready Pod and
// returns its name. It also returns the targetPort the service exposes so
// the caller does not have to ask separately.
//
// When portName is empty the first declared port is used — matching
// `kubectl port-forward service/X local:remote` ergonomics.
func (c *Client) ResolveServiceToPod(ctx context.Context, namespace, serviceName, portName string) (string, uint16, error) {
	svc, err := c.Clientset.CoreV1().Services(namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return "", 0, fmt.Errorf("get service: %w", err)
	}
	if len(svc.Spec.Selector) == 0 {
		// Headless or selector-less services have no backing pods we
		// can resolve. The user has to forward to a pod directly.
		return "", 0, fmt.Errorf("service %q has no selector; forward to a pod instead", serviceName)
	}

	targetPort, err := resolveServicePort(svc, portName)
	if err != nil {
		return "", 0, err
	}

	selector := labels.SelectorFromSet(svc.Spec.Selector).String()
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", 0, fmt.Errorf("list pods: %w", err)
	}
	pod, ok := firstReadyPod(pods.Items)
	if !ok {
		return "", 0, fmt.Errorf("no ready pod backs service %q", serviceName)
	}
	return pod, targetPort, nil
}

// ResolveDeploymentToPod returns the first ready pod produced by the
// deployment. We resolve via the deployment's pod-template label
// selector — same approach kubectl uses.
func (c *Client) ResolveDeploymentToPod(ctx context.Context, namespace, deployName string) (string, error) {
	dep, err := c.Clientset.AppsV1().Deployments(namespace).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get deployment: %w", err)
	}
	if dep.Spec.Selector == nil || len(dep.Spec.Selector.MatchLabels) == 0 {
		return "", fmt.Errorf("deployment %q has no matchLabels selector", deployName)
	}
	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels).String()
	pods, err := c.Clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return "", fmt.Errorf("list pods: %w", err)
	}
	pod, ok := firstReadyPod(pods.Items)
	if !ok {
		return "", fmt.Errorf("no ready pod for deployment %q", deployName)
	}
	return pod, nil
}

// resolveServicePort picks a port from the service spec: by name if the
// caller supplied one, otherwise the first declared port. Named ports
// resolve to their underlying targetPort number; raw numbers pass
// through. We intentionally do *not* support string targetPorts that
// resolve to container portNames — that would require a second lookup
// to the pod template, and the common case is numeric targetPorts.
func resolveServicePort(svc *corev1.Service, portName string) (uint16, error) {
	if len(svc.Spec.Ports) == 0 {
		return 0, fmt.Errorf("service %q exposes no ports", svc.Name)
	}
	pick := svc.Spec.Ports[0]
	if portName != "" {
		found := false
		for _, p := range svc.Spec.Ports {
			if p.Name == portName {
				pick = p
				found = true
				break
			}
		}
		if !found {
			return 0, fmt.Errorf("service %q has no port named %q", svc.Name, portName)
		}
	}
	if pick.TargetPort.IntValue() > 0 {
		return uint16(pick.TargetPort.IntValue()), nil //nolint:gosec // value clamped by k8s API to int32, range fits
	}
	if pick.Port > 0 {
		return uint16(pick.Port), nil //nolint:gosec // Service.Port is int32 in [0, 65535]
	}
	return 0, fmt.Errorf("service %q port %q has no usable number", svc.Name, pick.Name)
}

// firstReadyPod walks pods in declaration order and returns the first
// that is Running + Ready. Caller-visible side-effect: forwards prefer
// the alphabetically first replica that's healthy — deterministic and
// matches kubectl's behaviour.
func firstReadyPod(items []corev1.Pod) (string, bool) {
	for _, p := range items {
		if p.Status.Phase != corev1.PodRunning {
			continue
		}
		ready := false
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if ready {
			return p.Name, true
		}
	}
	return "", false
}
