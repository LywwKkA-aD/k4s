package forwards

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// statePath resolves the on-disk JSON location, honouring XDG_STATE_HOME
// only when it is absolute (the Base Directory spec says to ignore a
// relative one) and falling back to ~/.local/state. The directory is
// **not** created here — that's Save's job, so Load can be a true no-op
// on first run.
func statePath() (string, error) {
	if env := os.Getenv("XDG_STATE_HOME"); filepath.IsAbs(env) {
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

// loadFrom reads and checks the state at path: a file that does not parse
// or holds an invalid forward is an error.
func loadFrom(path string) (State, error) {
	raw, err := readState(path)
	if err != nil || len(raw) == 0 {
		return State{}, err
	}
	s, err := parseState(raw)
	if err != nil {
		return State{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// readState returns the file's bytes, none when it does not exist.
func readState(path string) ([]byte, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-controlled config path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return raw, nil
}

func parseState(raw []byte) (State, error) {
	var s State
	if err := json.Unmarshal(raw, &s); err != nil {
		return State{}, fmt.Errorf("parse: %w", err)
	}
	if err := s.validate(); err != nil {
		return State{}, err
	}
	return s, nil
}

// openState loads the state at path for a Manager. A corrupt file is moved
// aside to path.corrupt-<time>, so the user keeps it while k4s starts with
// no forwards, and notice says so. A file that cannot be read stays where
// it is and is an error.
func openState(path string, now time.Time) (s State, notice string, err error) {
	raw, err := readState(path)
	if err != nil || len(raw) == 0 {
		return State{}, "", err
	}
	s, corrupt := parseState(raw)
	if corrupt == nil {
		return s, "", nil
	}
	aside := corruptName(path, now)
	if err := os.Rename(path, aside); err != nil {
		return State{}, "", fmt.Errorf("set aside corrupt %s (%w): %w", path, corrupt, err)
	}
	return State{}, fmt.Sprintf("saved forwards were unreadable (%v); moved to %s", corrupt, displayPath(aside)), nil
}

// corruptName is where a corrupt state file goes: next to it, stamped
// with the UTC time down to the millisecond.
func corruptName(path string, now time.Time) string {
	return path + ".corrupt-" + now.UTC().Format("20060102T150405.000Z")
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

// saveTo writes s to path through a synced temp file and a rename, then
// syncs the directory so the rename itself survives a power cut. The
// directory is kept to its owner: the file names the user's namespaces,
// pods and clusters.
func (s State) saveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	if err := ownerOnly(dir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "portforwards-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if err := writeSynced(tmp, data); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename state: %w", err)
	}
	return syncDir(dir)
}

// ownerOnly strips group and other permissions from dir. MkdirAll leaves
// an existing directory's mode alone, and one created by an earlier k4s or
// another tool may be open to every user.
func ownerOnly(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("stat state dir: %w", err)
	}
	perm := info.Mode().Perm()
	if perm&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(dir, perm&0o700); err != nil {
		return fmt.Errorf("restrict state dir: %w", err)
	}
	return nil
}

func writeSynced(f *os.File, data []byte) error {
	_, err := f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write state: %w", err)
	}
	return nil
}

// syncDir flushes dir's entries. Windows cannot open a directory for
// writing, and NTFS journals the rename anyway.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir) //nolint:gosec // the state directory k4s owns
	if err != nil {
		return fmt.Errorf("open state dir: %w", err)
	}
	err = d.Sync()
	if closeErr := d.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("sync state dir: %w", err)
	}
	return nil
}

// displayPath shortens a path under the home directory to ~/... for
// messages.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	rel, ok := strings.CutPrefix(path, filepath.Clean(home)+string(filepath.Separator))
	if !ok {
		return path
	}
	return "~" + string(filepath.Separator) + rel
}
