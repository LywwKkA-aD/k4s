package k8s

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
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
// traffic to remotePort inside the named pod via SPDY. Returns a session
// the caller controls; do not call StartPodPortForward.ForwardPorts —
// that is what the background goroutine does.
//
// The forward dies when StopCh is closed *or* when the k4s process exits.
// State persistence lives a layer above (forwards.Manager).
//
// ctx is accepted for symmetry with the rest of the k8s package but is
// **not** wired to the session lifetime. Manager.Start uses ctx for the
// startup phase only — once Ready fires, the SPDY tunnel is governed by
// StopCh alone. (Earlier versions did bind ctx to stopCh; that killed
// forwards as soon as the caller's short-lived startup context expired.)
func (c *Client) StartPodPortForward(ctx context.Context, namespace, podName string, localPort, remotePort uint16) (*PortForwardSession, error) {
	_ = ctx
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
	// Capture forwarder output instead of routing it to the real
	// stdout/stderr — the TUI owns those, and a stray "Unable to
	// listen on port 80" banner from kubectl would shred the render.
	// Two separate buffers so the supervisor can tell info chatter
	// ("Forwarding from …" on stdout) apart from real failures
	// ("bind: permission denied" on stderr).
	stdout := &captureWriter{}
	stderr := &captureWriter{}
	fw, err := portforward.New(dialer, ports, stopCh, readyCh, stdout, stderr)
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

	// NOTE: we used to spawn a watcher that closed stopCh on
	// ctx.Done(). That tied the session's lifetime to the caller's
	// context — and callers typically pass a short-lived "startup"
	// context that gets cancelled the moment Manager.Start returns
	// (after Ready fires). The result was a forward that came up
	// healthy and was killed milliseconds later. Now the session
	// outlives ctx: Manager.Stop owns stopCh; ctx is only relevant
	// up to Ready/Err in Manager.Start's own select.

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
