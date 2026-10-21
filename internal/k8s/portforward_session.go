package k8s

import (
	"context"
	"fmt"
	"io"
	"time"

	"k8s.io/client-go/tools/portforward"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// PortForwardSession is the live handle for one in-process forward.
//
// Lifecycle:
//  1. Caller invokes StartPodPortForward — it returns immediately and a
//     goroutine drives the tunnel.
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
	// Stderr returns the last lines, at most 4 KiB, the forwarder wrote to
	// its error stream: failures such as "bind: permission denied". The
	// supervisor uses this to surface a useful detail when a session
	// dies. Its informational output ("Forwarding from …") is dropped.
	Stderr func() string
	// LastError returns the latest failure on one of the forward's
	// connections while it runs; the session itself keeps running. Nil
	// means the session does not report them.
	LastError func() TunnelError
	// Errors receives a value when LastError has news. A nil channel
	// never does.
	Errors <-chan struct{}
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
	serverURL, err := portForwardURL(c.RestConfig, namespace, podName)
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
	sess, err := runForwarder(dialer, podName, localPort, remotePort, stopCh, cancel, time.Now)
	if err != nil {
		cancel()
		return nil, err
	}
	return sess, nil
}

// runForwarder starts client-go's forwarder over dialer on its own
// goroutine and returns the session that controls it. release runs once
// the forwarder has returned.
func runForwarder(dialer streamhttp.Dialer, podName string, localPort, remotePort uint16, stopCh chan struct{}, release func(), now func() time.Time) (*PortForwardSession, error) {
	readyCh := make(chan struct{})
	errCh := make(chan error, 1)
	doneCh := make(chan struct{})
	tunnelErrs := newTunnelErrors(now)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}
	// The TUI owns the terminal, so the forwarder never writes to it. Its
	// chatter ("Handling connection for 8080", once per connection) is
	// dropped; its failures are kept, bounded, for the supervisor.
	stderr := &ring{}
	fw, err := portforward.NewForStreaming(observe(dialer, tunnelErrs), ports, stopCh, readyCh, io.Discard, stderr)
	if err != nil {
		return nil, fmt.Errorf("build forwarder: %w", err)
	}

	go func() {
		defer close(doneCh)
		defer release()
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
		StopCh:    stopCh,
		Ready:     readyCh,
		Err:       errCh,
		Done:      doneCh,
		Pod:       podName,
		Stderr:    stderr.String,
		LastError: tunnelErrs.last,
		Errors:    tunnelErrs.changed,
	}, nil
}
