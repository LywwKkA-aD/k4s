// Package forwards owns the lifecycle of port-forward "intents" — the
// user-declared list of forwards k4s should try to keep alive — plus the
// runtime Manager that turns them into live tunnels via client-go.
//
// In-process design: each running forward is a goroutine bound to a
// stopCh; the tunnel lives only for the lifetime of the k4s process. The
// State JSON file at $XDG_STATE_HOME/k4s/portforwards.json preserves the
// *intent*, so a restart can offer to revive every previously-running
// forward with one keystroke.
package forwards

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// Forward is one persisted port-forward intent. The ID is opaque and
// stable across restarts; tests rely on the comparison being structural,
// so add fields with care.
type Forward struct {
	ID      string `json:"id"`
	Context string `json:"context"`
	// Server and CA name the cluster behind Context when the forward was
	// saved: its API server address and the fingerprint of the CA the
	// client trusted. Forwards saved before k4s recorded them have neither.
	Server     string `json:"server,omitempty"`
	CA         string `json:"ca,omitempty"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"` // "pod" | "service" | "deployment"
	Name       string `json:"name"`
	LocalPort  uint16 `json:"local_port"`
	RemotePort uint16 `json:"remote_port"`
}

var (
	// validID matches what NewID makes and what earlier versions saved.
	validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	validCA = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// maxText bounds the free-form fields, context names and server
// addresses, which have no Kubernetes length rule of their own.
const maxText = 2048

// Validate rejects forwards that would refuse to start anyway, so the UI
// can show a useful error before we hit the API. Names follow the
// Kubernetes rules, which also keeps ".." and "/" out of the API paths
// they end up in.
func (f Forward) Validate() error {
	return f.validate(false)
}

// validate is Validate; a stored forward may also be one saved before k4s
// recorded servers, whose context may be empty.
func (f Forward) validate(stored bool) error {
	switch {
	case f.ID == "":
		return errors.New("forward id is empty")
	case !validID.MatchString(f.ID):
		return fmt.Errorf("forward id %q is not a k4s id", f.ID)
	case f.Context == "" && (!stored || f.Server != ""):
		return errors.New("context is empty")
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
	return f.validateText()
}

// validateText checks the fields that reach API paths or the screen.
func (f Forward) validateText() error {
	if err := inert("context", f.Context); err != nil {
		return err
	}
	if err := inert("server", f.Server); err != nil {
		return err
	}
	if f.CA != "" && !validCA.MatchString(f.CA) {
		return fmt.Errorf("ca %q is not a sha256 fingerprint", f.CA)
	}
	if err := kubectl.CheckLabel("namespace", f.Namespace); err != nil {
		return err
	}
	return kubectl.CheckSubdomain(f.Kind, f.Name)
}

// inert rejects text that would not draw as itself: control characters,
// escape sequences, invalid UTF-8, or more than maxText bytes.
func inert(field, s string) error {
	if len(s) > maxText || sanitize.Line(s) != s {
		return fmt.Errorf("%s %q is not plain text", field, s)
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

// validate checks every stored forward and that no id repeats.
func (s State) validate() error {
	seen := make(map[string]bool, len(s.Forwards))
	for i, f := range s.Forwards {
		if err := f.validate(true); err != nil {
			return fmt.Errorf("forward %d: %w", i+1, err)
		}
		if seen[f.ID] {
			return fmt.Errorf("forward %d: duplicate id %q", i+1, f.ID)
		}
		seen[f.ID] = true
	}
	return nil
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
