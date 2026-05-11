package forwards

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// Status is the runtime state of one forward inside this Manager.
//
// The transition graph is:
//
//	Stopped ── Start() ──▶ Starting ── ready ──▶ Running
//	             │                       │
//	             ▼                       ▼
//	          Error ◀──── error/exit ───┘
type Status int

const (
	// StatusStopped is the default — declared in state, no goroutine.
	StatusStopped Status = iota
	StatusStarting
	StatusRunning
	StatusError
)

// String renders the status for the UI table.
func (s Status) String() string {
	switch s {
	case StatusStopped:
		return "stopped"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusError:
		return "error"
	}
	return "unknown"
}

// Active is one entry in the Manager's runtime list: the persisted
// Forward plus its current Status and last error (if any). The session
// pointer stays private — callers go through Manager.Stop / Manager.Start
// rather than poking at it directly.
type Active struct {
	Forward Forward
	Status  Status
	Err     error
	session *k8s.PortForwardSession
}

// Manager owns both the on-disk State and the in-memory runtime map.
//
// Mutability invariant: every read returns a fresh copy (the *Active
// struct is value-copied) so the UI never holds a pointer into the
// internal map. Writes happen under Manager.mu.
type Manager struct {
	mu     sync.Mutex
	client *k8s.Client
	state  State
	active map[string]*Active
	// changeCh is closed and re-created on every mutation so a single
	// UI goroutine can `select` on it and refresh. Channels feel more
	// idiomatic here than condvars or callbacks.
	changeCh chan struct{}
}

// NewManager loads persisted state and returns a fresh Manager. Every
// known intent starts in StatusStopped — we do not auto-revive forwards
// on launch, the user explicitly opts in via the forwards view.
func NewManager(client *k8s.Client) (*Manager, error) {
	state, err := Load()
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}
	m := &Manager{
		client:   client,
		state:    state,
		active:   make(map[string]*Active),
		changeCh: make(chan struct{}),
	}
	for _, f := range state.Forwards {
		m.active[f.ID] = &Active{Forward: f, Status: StatusStopped}
	}
	return m, nil
}

// Changes returns a channel that closes the next time the Manager mutates
// state. Callers `<-mgr.Changes()` to wait, then re-read List() and call
// Changes() again. The channel is one-shot by design — race-free idiom.
func (m *Manager) Changes() <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.changeCh
}

func (m *Manager) notify() {
	close(m.changeCh)
	m.changeCh = make(chan struct{})
}

// List returns a snapshot copy of every known forward, sorted by
// (Namespace, Kind, Name, LocalPort) for a stable display order — the
// UI's selection cursor would otherwise jump on every render since
// map iteration is undefined.
func (m *Manager) List() []Active {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Active, 0, len(m.active))
	for _, a := range m.active {
		out = append(out, Active{Forward: a.Forward, Status: a.Status, Err: a.Err})
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].Forward, out[j].Forward
		if ai.Namespace != aj.Namespace {
			return ai.Namespace < aj.Namespace
		}
		if ai.Kind != aj.Kind {
			return ai.Kind < aj.Kind
		}
		if ai.Name != aj.Name {
			return ai.Name < aj.Name
		}
		return ai.LocalPort < aj.LocalPort
	})
	return out
}

// State returns the persisted intent. Useful for restore prompts at
// startup or sanity-check tests; not what the UI uses for rendering.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// NewID generates a short opaque ID for a new forward. Eight bytes of
// hex (16 chars) — fits a UI column, low enough collision probability.
func NewID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to a timestamp; uniqueness is best-effort here.
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// Register adds (or updates) a forward in the State and pushes the
// stopped entry into the runtime map. It does not start the forward.
// The caller is expected to chain Start when they want it live.
func (m *Manager) Register(f Forward) error {
	if err := f.Validate(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = m.state.Upsert(f)
	if existing, ok := m.active[f.ID]; ok {
		existing.Forward = f
	} else {
		m.active[f.ID] = &Active{Forward: f, Status: StatusStopped}
	}
	if err := m.state.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	m.notify()
	return nil
}

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

	if err := k8s.CheckLocalPortAvailable(fwd.LocalPort); err != nil {
		friendly := friendlyBindError(err, fwd.LocalPort)
		m.markError(id, friendly)
		return friendly
	}

	pod, err := m.resolveTarget(ctx, fwd)
	if err != nil {
		m.markError(id, fmt.Errorf("resolve target: %w", err))
		return err
	}

	sess, err := m.client.StartPodPortForward(ctx, fwd.Namespace, pod, fwd.LocalPort, fwd.RemotePort)
	if err != nil {
		m.markError(id, fmt.Errorf("start forward: %w", err))
		return err
	}

	// Wait either for ready, an error, or the session to die before
	// transitioning into a steady state. Whichever fires first wins.
	select {
	case <-sess.Ready:
		m.mu.Lock()
		if cur, ok := m.active[id]; ok {
			cur.Status = StatusRunning
			cur.session = sess
		}
		m.notify()
		m.mu.Unlock()
	case err := <-sess.Err:
		sess.Close()
		m.markError(id, err)
		return err
	case <-sess.Done:
		m.markError(id, errors.New("forward terminated before ready"))
		return errors.New("forward terminated before ready")
	case <-ctx.Done():
		sess.Close()
		m.markError(id, ctx.Err())
		return ctx.Err()
	}

	// Background supervisor: watch for unexpected termination so the
	// UI can flip the status to error without a manual refresh.
	go m.supervise(id, sess)
	return nil
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
			m.markError(id, decorateError(err, sess.Stderr))
			return
		}
	}

	// Done landed first. Drain a pending Err if it's already queued so
	// we don't lose the real reason ("lost connection to pod", ...).
	select {
	case err, ok := <-sess.Err:
		if ok && err != nil {
			m.markError(id, decorateError(err, sess.Stderr))
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
			m.markError(id, errors.New(condensed))
			return
		}
	}

	// Clean stop — user closed the session via Stop(), or k8s closed
	// the SPDY connection without complaint.
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.active[id]; ok && cur.Status == StatusRunning {
		cur.Status = StatusStopped
		cur.session = nil
		m.notify()
	}
}

// decorateError appends the captured stderr from the portforwarder to
// the raw error when it carries useful detail. The unwrapped error
// from ForwardPorts is often just "lost connection to pod" — the real
// reason ("bind: permission denied") lives in the captured output.
func decorateError(err error, output func() string) error {
	if output == nil {
		return err
	}
	extra := strings.TrimSpace(output())
	if extra == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, condense(extra))
}

// condense trims long stderr blobs to one line that is actually useful
// in a status column. Looks for well-known bind errors first; falls
// back to the first non-empty line.
func condense(s string) string {
	for _, sub := range []string{"bind: permission denied", "bind: address already in use"} {
		if strings.Contains(s, sub) {
			return sub
		}
	}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return s
}

// friendlyBindError translates a raw net.Listen error into a one-liner
// the user can act on. Falls through with a generic message when the
// OS does not surface a recognised string.
func friendlyBindError(err error, port uint16) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "permission denied"):
		return fmt.Errorf("permission denied binding local port %d (ports < 1024 need root)", port)
	case strings.Contains(s, "address already in use"):
		return fmt.Errorf("local port %d already in use", port)
	}
	return fmt.Errorf("cannot bind local port %d: %w", port, err)
}

func (m *Manager) markError(id string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cur, ok := m.active[id]; ok {
		cur.Status = StatusError
		cur.Err = err
		cur.session = nil
		m.notify()
	}
}

// Stop signals the running session to terminate. It does NOT remove the
// forward from State — the user still wants to remember the intent.
// No-op if the forward is already stopped or in error.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	a, ok := m.active[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("forward %q not found", id)
	}
	sess := a.session
	if sess == nil {
		// Nothing live to stop; just normalise status if it was Error.
		a.Status = StatusStopped
		a.Err = nil
		m.notify()
		m.mu.Unlock()
		return nil
	}
	a.Status = StatusStopped
	a.session = nil
	m.notify()
	m.mu.Unlock()
	sess.Close()
	return nil
}

// Remove stops the forward and deletes it from State.
func (m *Manager) Remove(id string) error {
	if err := m.Stop(id); err != nil {
		// Don't abort — the intent should still be removed even if we
		// failed to clean up a phantom session.
		_ = err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.active, id)
	m.state = m.state.Remove(id)
	if err := m.state.Save(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	m.notify()
	return nil
}

// resolveTarget turns the kind+name into a concrete pod name. Pod kinds
// pass through; Service and Deployment go through their respective
// resolvers in the k8s package.
func (m *Manager) resolveTarget(ctx context.Context, f Forward) (string, error) {
	switch f.Kind {
	case "pod":
		return f.Name, nil
	case "service":
		pod, _, err := m.client.ResolveServiceToPod(ctx, f.Namespace, f.Name, "")
		return pod, err
	case "deployment":
		return m.client.ResolveDeploymentToPod(ctx, f.Namespace, f.Name)
	}
	return "", fmt.Errorf("unknown kind %q", f.Kind)
}
