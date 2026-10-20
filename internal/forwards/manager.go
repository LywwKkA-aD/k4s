package forwards

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
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
	mu            sync.Mutex
	client        *k8s.Client
	contextFilter string
	state         State
	active        map[string]*Active
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
	ctxName := ""
	if client != nil {
		ctxName = client.Context
	}
	m := &Manager{
		client:        client,
		contextFilter: ctxName,
		state:         state,
		active:        make(map[string]*Active),
		changeCh:      make(chan struct{}),
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

// List returns a snapshot copy of every known forward for the active
// context, sorted by (Namespace, Kind, Name, LocalPort) for a stable
// display order — the UI's selection cursor would otherwise jump on every
// render since map iteration is undefined.
func (m *Manager) List() []Active {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Active, 0, len(m.active))
	for _, a := range m.active {
		if !m.matchesContext(a.Forward.Context) {
			continue
		}
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

func (m *Manager) matchesContext(ctx string) bool {
	return m.contextFilter == "" || ctx == "" || ctx == m.contextFilter
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
	select {
	case <-sess.Done:
	case <-time.After(2 * time.Second):
	}
	return nil
}

// StopAll terminates every live forward and waits for their listeners to
// close. Use this when switching contexts so the old cluster's forwards do
// not pin local ports after the user has moved on.
func (m *Manager) StopAll() error {
	m.mu.Lock()
	var sessions []*k8s.PortForwardSession
	for _, a := range m.active {
		if a.session != nil {
			sessions = append(sessions, a.session)
		}
		a.Status = StatusStopped
		a.session = nil
		a.Err = nil
	}
	m.notify()
	m.mu.Unlock()

	for _, sess := range sessions {
		sess.Close()
	}
	for _, sess := range sessions {
		select {
		case <-sess.Done:
		case <-time.After(2 * time.Second):
		}
	}
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
