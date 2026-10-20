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
// goroutine; Start returns once Ready fires or an error surfaces.
//
// A local-port pre-check runs *before* we open the SPDY tunnel so the
// user gets a clear, immediate error message — "permission denied for
// port 80", "address already in use" — instead of a wall of kubectl
// stderr leaking through the TUI.
func (m *Manager) Start(ctx context.Context, id string) error {
	m.mu.Lock()
	a, ok := m.active[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("forward %q not found", id)
	}
	if a.Status == StatusRunning || a.Status == StatusStarting {
		m.mu.Unlock()
		return errors.New("already running")
	}
	a.Status = StatusStarting
	a.Err = nil
	fwd := a.Forward
	m.notify()
	m.mu.Unlock()

	if m.tunnel == nil {
		m.markError(id, errors.New("no kubernetes client"))
		return errors.New("no kubernetes client")
	}

	if err := m.probePort(fwd.LocalPort); err != nil {
		friendly := friendlyBindError(err, fwd.LocalPort)
		m.markError(id, friendly)
		return friendly
	}

	pod, remotePort, err := m.resolveTarget(ctx, fwd)
	if err != nil {
		m.markError(id, fmt.Errorf("resolve target: %w", err))
		return err
	}

	sess, err := m.tunnel.StartPodPortForward(ctx, fwd.Namespace, pod, fwd.LocalPort, remotePort)
	if err != nil {
		m.markError(id, fmt.Errorf("start forward: %w", err))
		return err
	}

	if !m.commitSession(id, sess) {
		return nil
	}
	if err := m.waitForReady(ctx, id, sess); err != nil {
		return err
	}

	// Background supervisor: watch for unexpected termination so the
	// UI can flip the status to error without a manual refresh.
	go m.supervise(id, sess)
	return nil
}

// commitSession stores the session handle while still in StatusStarting so
// Stop/Remove can tear it down. Returns false if the forward was cancelled
// before the session could be committed, in which case the session is closed.
func (m *Manager) commitSession(id string, sess *k8s.PortForwardSession) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.active[id]
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
func (m *Manager) waitForReady(ctx context.Context, id string, sess *k8s.PortForwardSession) error {
	select {
	case <-sess.Ready:
		m.mu.Lock()
		cur, ok := m.active[id]
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
		m.markErrorWithSession(id, sess, decorateError(err, sess.Stderr))
		return err
	case <-sess.Done:
		m.markErrorWithSession(id, sess, errors.New("forward terminated before ready"))
		return errors.New("forward terminated before ready")
	case <-ctx.Done():
		sess.Close()
		m.markErrorWithSession(id, sess, ctx.Err())
		return ctx.Err()
	}
}

func (m *Manager) supervise(id string, sess *k8s.PortForwardSession) {
	// First leg: wait for either Done or an explicit error. fw.ForwardPorts
	// signals failure by both writing to Err *and* closing Done, so Go's
	// random select tie-breaking can drop us into either case — that's why
	// the rest of this function handles both paths.
	select {
	case <-sess.Done:
	case err, ok := <-sess.Err:
		if ok && err != nil {
			m.markErrorWithSession(id, sess, decorateError(err, sess.Stderr))
			return
		}
	}

	// Done landed first. Drain a pending Err if it's already queued so
	// we don't lose the real reason ("lost connection to pod", ...).
	select {
	case err, ok := <-sess.Err:
		if ok && err != nil {
			m.markErrorWithSession(id, sess, decorateError(err, sess.Stderr))
			return
		}
	default:
	}

	// No explicit Err, but the forwarder may still have written useful
	// diagnostics to its stderr capture before the session collapsed.
	// We deliberately only check stderr here — stdout carries
	// informational chatter ("Forwarding from 127.0.0.1:8080 -> 80")
	// which is *not* an error and shouldn't flip the status to red.
	if sess.Stderr != nil {
		if condensed := condense(strings.TrimSpace(sess.Stderr())); condensed != "" {
			m.markErrorWithSession(id, sess, errors.New(condensed))
			return
		}
	}

	// Clean stop — user closed the session via Stop(), or k8s closed
	// the SPDY connection without complaint. Guard against a stale
	// supervise goroutine from a previous session for the same ID.
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.active[id]; ok && cur.session == sess && cur.Status == StatusRunning {
		cur.Status = StatusStopped
		cur.session = nil
		m.notify()
	}
}

func (m *Manager) markError(id string, err error) {
	m.markErrorWithSession(id, nil, err)
}

func (m *Manager) markErrorWithSession(id string, sess *k8s.PortForwardSession, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.active[id]
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
