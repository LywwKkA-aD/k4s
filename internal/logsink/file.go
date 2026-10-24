package logsink

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// maxLogSize is the size at which Open moves the log aside to k4s.log.1.
const maxLogSize = 5 << 20

const (
	dirPerm  fs.FileMode = 0o700
	filePerm fs.FileMode = 0o600
)

// openLogFile opens path for appending in a private directory, rotating it
// first if it has reached maxLogSize. Errors and API warnings can name
// hosts, namespaces and users, so the directory is 0700 and the file 0600
// even when they already existed with looser modes, and a symbolic link
// at path is refused where the platform can (see noFollow).
func openLogFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	if err := tighten(dir, dirPerm); err != nil {
		return nil, err
	}
	if err := rotate(path); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND|noFollow, filePerm)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}
	if err := tighten(path, filePerm); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// tighten drops any permission bits beyond want.
func tighten(path string, want fs.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", filepath.Base(path), err)
	}
	if info.Mode().Perm()&^want == 0 {
		return nil
	}
	if err := os.Chmod(path, want); err != nil {
		return fmt.Errorf("restrict %s: %w", filepath.Base(path), err)
	}
	return nil
}

// rotate moves path to path.1 (replacing it) once path reaches maxLogSize.
func rotate(path string) error {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat log file: %w", err)
	}
	if info.Size() < maxLogSize {
		return nil
	}
	// Windows refuses to rename onto an existing file.
	if err := os.Remove(path + ".1"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove old rotated log: %w", err)
	}
	if err := os.Rename(path, path+".1"); err != nil {
		return fmt.Errorf("rotate log file: %w", err)
	}
	return nil
}
