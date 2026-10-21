package k8s

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/portforward"
)

// tryListen abstracts the listener creation so port-availability checks
// can be exercised in tests without poking at real OS ports.
var tryListen = func(port uint16) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
}

// captureWriter is the io.Writer we hand to portforward.PortForwarder
// instead of os.Stdout/Stderr. The bubbletea TUI owns the real
// terminal; if the forwarder wrote there directly it would smear its
// "Unable to listen on port X" banner across the rendered table. Here
// we buffer the bytes and let the supervisor surface them via
// PortForwardSession.Output once the session ends.
type captureWriter struct {
	mu  sync.Mutex
	buf strings.Builder
}

// Write implements io.Writer.
func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

// String returns the captured output so far.
func (w *captureWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

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
	// Stderr returns whatever the forwarder wrote to its stderr stream.
	// kubectl prints actual failures here ("bind: permission denied",
	// "lost connection to pod", …). The supervisor uses this to
	// surface a useful detail when a session dies.
	Stderr func() string
	// Stdout returns the stream the forwarder uses for informational
	// chatter ("Forwarding from 127.0.0.1:8080 -> 80"). Almost never
	// useful for error reporting — kept around for completeness and
	// future "show me the boring details" affordances.
	Stdout func() string
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
// traffic to remotePort inside the named pod. Returns a session the caller
// controls; the forwarder itself runs on a goroutine.
//
// The forward dies when StopCh is closed *or* when the k4s process exits.
// State persistence lives a layer above (forwards.Manager).
//
// ctx is accepted for symmetry with the rest of the k8s package but is not
// wired to the session: callers pass a short "startup" context that ends
// once Start returns, and a session bound to it would die right after it
// came up. The dial has its own bound, portForwardStartup, which StopCh
// cuts short.
func (c *Client) StartPodPortForward(ctx context.Context, namespace, podName string, localPort, remotePort uint16) (*PortForwardSession, error) {
	_ = ctx
	return c.startPodPortForward(namespace, podName, localPort, remotePort, portForwardStartup)
}

// startPodPortForward is StartPodPortForward with the dial bounded by
// startup instead of portForwardStartup.
func (c *Client) startPodPortForward(namespace, podName string, localPort, remotePort uint16, startup time.Duration) (*PortForwardSession, error) {
	if c.RestConfig == nil {
		return nil, fmt.Errorf("client built without rest config; port-forward is unavailable")
	}
	serverURL, err := parsePortForwardURL(c.RestConfig.Host, namespace, podName)
	if err != nil {
		return nil, fmt.Errorf("build portforward URL: %w", err)
	}

	stopCh := make(chan struct{})
	startCtx, cancel := startupContext(stopCh, startup)
	dialer, err := newPortForwardDialer(withStartupContext(startCtx, c.RestConfig), serverURL)
	if err != nil {
		cancel()
		return nil, err
	}

	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	// Capture forwarder output instead of routing it to the real
	// stdout/stderr — the TUI owns those, and a stray "Unable to
	// listen on port 80" banner from kubectl would shred the render.
	// Two separate buffers so the supervisor can tell info chatter
	// ("Forwarding from …" on stdout) apart from real failures
	// ("bind: permission denied" on stderr).
	stdout := &captureWriter{}
	stderr := &captureWriter{}
	fw, err := portforward.NewForStreaming(dialer, ports, stopCh, readyCh, stdout, stderr)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("build forwarder: %w", err)
	}

	go func() {
		defer close(doneCh)
		defer cancel()
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

	return &PortForwardSession{
		StopCh: stopCh,
		Ready:  readyCh,
		Err:    errCh,
		Done:   doneCh,
		Pod:    podName,
		Stdout: stdout.String,
		Stderr: stderr.String,
	}, nil
}

// parsePortForwardURL parses RestConfig.Host into a URL that preserves any
// path prefix (e.g. Rancher clusters at /k8s/clusters/c-xxx) and scheme
// (https or http) instead of hardcoding https and dropping the path.
func parsePortForwardURL(host, namespace, podName string) (*url.URL, error) {
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName))
	return u, nil
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
