// Package forwards owns the lifecycle of port-forward "intents" — the
// user-declared list of forwards k4s should try to keep alive — plus the
// runtime Manager that turns them into live SPDY tunnels via client-go.
//
// In-process design: each running forward is a goroutine bound to a
// stopCh; the SPDY connection lives only for the lifetime of the k4s
// process. The State JSON file at $XDG_STATE_HOME/k4s/portforwards.json
// preserves the *intent*, so a restart can offer to revive every
// previously-running forward with one keystroke.
package forwards

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Forward is one persisted port-forward intent. The ID is opaque and
// stable across restarts; tests rely on the comparison being structural,
// so add fields with care.
type Forward struct {
	ID         string `json:"id"`
	Context    string `json:"context"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"` // "pod" | "service" | "deployment"
	Name       string `json:"name"`
	LocalPort  uint16 `json:"local_port"`
	RemotePort uint16 `json:"remote_port"`
}

// Validate rejects forwards that would refuse to start anyway, so the UI
// can show a useful error before we hit the API.
func (f Forward) Validate() error {
	switch {
	case f.ID == "":
		return errors.New("forward id is empty")
	case f.Namespace == "":
		return errors.New("namespace is empty")
	case f.Name == "":
		return errors.New("name is empty")
	case !validKind(f.Kind):
		return fmt.Errorf("unknown kind %q (want pod, service or deployment)", f.Kind)
	case f.LocalPort == 0:
		return errors.New("local port must be > 0")
	case f.RemotePort == 0:
		return errors.New("remote port must be > 0")
	}
	return nil
}

func validKind(k string) bool {
	switch k {
	case "pod", "service", "deployment":
		return true
	}
	return false
}

// State is the wire format on disk. A struct-with-slice (rather than a
// bare slice) leaves room for future top-level fields — e.g. version,
// per-context preferences — without breaking the file.
type State struct {
	Forwards []Forward `json:"forwards"`
}

// FindByID returns a copy of the matching forward and true, or zero and
// false. Forwards are typically small so a linear scan is fine.
func (s State) FindByID(id string) (Forward, bool) {
	for _, f := range s.Forwards {
		if f.ID == id {
			return f, true
		}
	}
	return Forward{}, false
}

// Upsert replaces an existing forward with the same ID, or appends if
// not found. Returns a new State — callers explicitly persist with Save.
func (s State) Upsert(f Forward) State {
	out := State{Forwards: make([]Forward, len(s.Forwards))}
	copy(out.Forwards, s.Forwards)
	for i, existing := range out.Forwards {
		if existing.ID == f.ID {
			out.Forwards[i] = f
			return out
		}
	}
	out.Forwards = append(out.Forwards, f)
	return out
}

// Remove drops the forward with the given ID. No-op if not present.
func (s State) Remove(id string) State {
	out := State{Forwards: make([]Forward, 0, len(s.Forwards))}
	for _, f := range s.Forwards {
		if f.ID == id {
			continue
		}
		out.Forwards = append(out.Forwards, f)
	}
	return out
}

// statePath resolves the on-disk JSON location, honouring XDG_STATE_HOME
// (per the Base Directory spec) and falling back to ~/.local/state. The
// directory is **not** created here — that's Save's job, so Load can be
// a true no-op on first run.
func statePath() (string, error) {
	if env := strings.TrimSpace(os.Getenv("XDG_STATE_HOME")); env != "" {
		return filepath.Join(env, "k4s", "portforwards.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home: %w", err)
	}
	return filepath.Join(home, ".local", "state", "k4s", "portforwards.json"), nil
}

// Load reads the persisted state. A missing file returns an empty State
// and a nil error — first-run users should not see a phantom failure.
func Load() (State, error) {
	path, err := statePath()
	if err != nil {
		return State{}, err
	}
	return loadFrom(path)
}

func loadFrom(path string) (State, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled config path
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, nil
		}
		return State{}, fmt.Errorf("read %s: %w", path, err)
	}
	if len(raw) == 0 {
		return State{}, nil
	}
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// Save writes the state atomically: temp file in the same directory,
// then rename. Crashing mid-write therefore leaves the previous version
// intact instead of a half-truncated file.
func (s State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	return s.saveTo(path)
}

func (s State) saveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "portforwards-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write state: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close state: %w", closeErr)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
