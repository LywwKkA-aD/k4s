package forwards

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// Start opens the SPDY tunnel for the given ID. Resolves Service or
// Deployment to a backing Pod when needed. The forward runs in a
// goroutine; Start returns once Ready fires or an error surfaces. A
// returned error is the same one the forward's status shows, except
// ErrStopping while the Manager is Blocked, which leaves the forward as it
// was.
func (m *Manager) Start(ctx context.Context, id string) (err error) {
	if err := m.Blocked(); err != nil {
		return err
	}
	fwd, attempt, err := m.beginAttempt(id)
	if err != nil {
		return err
	}
	defer m.recoverStart(id, attempt, &err)

	sess, err := m.openTunnel(ctx, fwd)
	if err != nil {
		m.markError(id, attempt, nil, err)
		return err
	}

	if !m.commitSession(id, attempt, sess) {
		return nil
	}
	if err := m.waitForReady(ctx, id, attempt, sess); err != nil {
		return err
	}

	// Background supervisor: watch for unexpected termination so the
	// UI can flip the status to error without a manual refresh.
	go m.supervise(id, attempt, sess)
	return nil
}

// beginAttempt moves the forward to StatusStarting and numbers this Start.
// A forward that may belong to another cluster is refused as it stands.
func (m *Manager) beginAttempt(id string) (Forward, uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.active[id]
	if !ok {
		return Forward{}, 0, notFound(id)
	}
	if err := m.refuseForeign(a.Forward); err != nil {
		return Forward{}, 0, err
	}
	if a.Status == StatusRunning || a.Status == StatusStarting {
		return Forward{}, 0, errors.New("already running")
	}
	a.Status = StatusStarting
	a.Err = nil
	a.ended = k8s.TunnelError{}
	m.attempts++
	a.attempt = m.attempts
	m.notify()
	return a.Forward, a.attempt, nil
}

// openTunnel runs every step up to a session handle. The local-port
// pre-check comes first so the user gets a clear, immediate error message
// — "permission denied for port 80", "address already in use" — instead
// of a wall of kubectl stderr leaking through the TUI.
func (m *Manager) openTunnel(ctx context.Context, fwd Forward) (*k8s.PortForwardSession, error) {
	if m.tunnel == nil {
		return nil, errors.New("no kubernetes client")
	}
	if err := m.probePort(fwd.LocalPort); err != nil {
		return nil, errors.New(FriendlyBindError(err, fwd.LocalPort))
	}
	pod, remotePort, err := m.resolveTarget(ctx, fwd)
	if err != nil {
		return nil, fmt.Errorf("resolve target: %w", err)
	}
	sess, err := m.tunnel.StartPodPortForward(ctx, fwd.Namespace, pod, fwd.LocalPort, remotePort)
	if err != nil {
		return nil, fmt.Errorf("start forward: %w", err)
	}
	return sess, nil
}

// owned returns the forward when attempt is still its latest Start.
// The caller holds m.mu.
func (m *Manager) owned(id string, attempt uint64) (*Active, bool) {
	cur, ok := m.active[id]
	if !ok || cur.attempt != attempt {
		return nil, false
	}
	return cur, true
}

// commitSession stores the session handle while still in StatusStarting so
// Stop/Remove can tear it down. Returns false if the forward was cancelled
// or restarted before the session could be committed, in which case the
// session is closed.
func (m *Manager) commitSession(id string, attempt uint64, sess *k8s.PortForwardSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.owned(id, attempt)
	if !ok || cur.Status != StatusStarting {
		go func() {
			sess.Close()
			select {
			case <-sess.Done:
			case <-time.After(2 * time.Second):
			}
		}()
		return false
	}
	cur.session = sess
	return true
}

// waitForReady blocks until the session signals ready, errors, or the caller
// context expires. It promotes StatusStarting to StatusRunning on success.
func (m *Manager) waitForReady(ctx context.Context, id string, attempt uint64, sess *k8s.PortForwardSession) error {
	select {
	case <-sess.Ready:
		m.mu.Lock()
		cur, ok := m.owned(id, attempt)
		if !ok || cur.Status != StatusStarting || cur.session != sess {
			m.mu.Unlock()
			sess.Close()
			return nil
		}
		cur.Status = StatusRunning
		m.notify()
		m.mu.Unlock()
		return nil
	case err := <-sess.Err:
		sess.Close()
		failed := decorateError(err, sess.Stderr)
		m.markError(id, attempt, sess, failed)
		return failed
	case <-sess.Done:
		failed := errors.New("forward terminated before ready")
		m.markError(id, attempt, sess, failed)
		return failed
	case <-ctx.Done():
		sess.Close()
		m.markError(id, attempt, sess, ctx.Err())
		return ctx.Err()
	}
}

func (m *Manager) supervise(id string, attempt uint64, sess *k8s.PortForwardSession) {
	defer m.recoverSupervisor(id, attempt, sess)
	// First leg: wait for either Done or an explicit error. fw.ForwardPorts
	// signals failure by both writing to Err *and* closing Done, so Go's
	// random select tie-breaking can drop us into either case — that's why
	// the rest of this function handles both paths.
	if err := m.awaitEnd(id, attempt, sess); err != nil {
		m.markError(id, attempt, sess, decorateError(err, sess.Stderr))
		return
	}

	// Done landed first. Drain a pending Err if it's already queued so
	// we don't lose the real reason ("lost connection to pod", ...).
	select {
	case err, ok := <-sess.Err:
		if ok && err != nil {
			m.markError(id, attempt, sess, decorateError(err, sess.Stderr))
			return
		}
	default:
	}

	// No explicit Err, but the forwarder may still have written useful
	// diagnostics to its stderr capture before the session collapsed.
	if sess.Stderr != nil {
		if condensed := condense(strings.TrimSpace(sess.Stderr())); condensed != "" {
			m.markError(id, attempt, sess, errors.New(condensed))
			return
		}
	}

	// Clean stop — user closed the session via Stop(), or k8s closed
	// the SPDY connection without complaint. Guard against a stale
	// supervise goroutine from a previous session for the same ID.
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.owned(id, attempt); ok && cur.session == sess && cur.Status == StatusRunning {
		cur.Status = StatusStopped
		cur.session = nil
		m.notify()
	}
}

// awaitEnd blocks until the session ends and returns the error it
// reported, if it reported one first. A failure on one of its connections
// in the meantime refreshes the list, which shows it while the forward
// keeps running.
func (m *Manager) awaitEnd(id string, attempt uint64, sess *k8s.PortForwardSession) error {
	for {
		select {
		case <-sess.Done:
			return nil
		case err := <-sess.Err:
			return err
		case <-sess.Errors:
			m.mu.Lock()
			if cur, ok := m.owned(id, attempt); ok && cur.session == sess {
				m.notify()
			}
			m.mu.Unlock()
		}
	}
}

// markError flips the forward to StatusError unless the report is stale:
// it comes from an older Start attempt, from a session the forward no
// longer holds (sess is nil before a session exists), or the user has
// stopped the forward since.
func (m *Manager) markError(id string, attempt uint64, sess *k8s.PortForwardSession, err error) {
	m.fail(id, attempt, sess, err, lastTunnelError(sess))
}

// fail is markError with the session's last connection failure read
// already, so a session that panics on that read cannot reach here.
func (m *Manager) fail(id string, attempt uint64, sess *k8s.PortForwardSession, err error, ended k8s.TunnelError) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.owned(id, attempt)
	if !ok {
		return
	}
	if sess != nil && cur.session != sess {
		return
	}
	if cur.Status == StatusStopped {
		return
	}
	cur.Status = StatusError
	cur.Err = err
	if sess != nil {
		cur.ended = ended
		cur.session = nil
	}
	m.notify()
}

// resolveTarget turns the kind+name into a concrete pod name and the
// container port the tunnel should target. Pods and deployments use the
// persisted RemotePort as is. For services RemotePort is a Service port, as
// in `kubectl port-forward svc/X LOCAL:REMOTE`, and is translated to that
// port's targetPort on the chosen pod.
func (m *Manager) resolveTarget(ctx context.Context, f Forward) (string, uint16, error) {
	switch f.Kind {
	case "pod":
		return f.Name, f.RemotePort, nil
	case "service":
		pod, target, err := m.tunnel.ResolveServiceToPod(ctx, f.Namespace, f.Name, int32(f.RemotePort))
		if err != nil {
			return "", 0, err
		}
		port, err := portNumber(target)
		if err != nil {
			return "", 0, fmt.Errorf("service %q port %d on pod %q: %w", f.Name, f.RemotePort, pod.Name, err)
		}
		return pod.Name, port, nil
	case "deployment":
		pod, err := m.tunnel.ResolveDeploymentToPod(ctx, f.Namespace, f.Name)
		if err != nil {
			return "", 0, err
		}
		return pod, f.RemotePort, nil
	}
	return "", 0, fmt.Errorf("unknown kind %q", f.Kind)
}

// portNumber narrows a Kubernetes int32 port to a TCP port number.
func portNumber(p int32) (uint16, error) {
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port %d is outside 1-65535", p)
	}
	return uint16(p), nil
}
