package logsink

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func skipPermsOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits")
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestOpenLogFileCreatesPrivateDirAndFile(t *testing.T) {
	t.Parallel()
	skipPermsOnWindows(t)

	dir := filepath.Join(t.TempDir(), "state", "k4s")
	path := filepath.Join(dir, "k4s.log")
	f, err := openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if got := mode(t, dir); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}
	if got := mode(t, path); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}
}

func TestOpenLogFileTightensExistingPerms(t *testing.T) {
	t.Parallel()
	skipPermsOnWindows(t)

	dir := filepath.Join(t.TempDir(), "k4s")
	path := filepath.Join(dir, "k4s.log")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Loosen after creation so the process umask cannot mask the setup.
	if err := os.Chmod(dir, 0o755); err != nil { //nolint:gosec // test loosens perms on purpose
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil { //nolint:gosec // test loosens perms on purpose
		t.Fatal(err)
	}

	f, err := openLogFile(path)
	if err != nil {
		t.Fatalf("openLogFile: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if got := mode(t, dir); got != 0o700 {
		t.Errorf("dir mode = %o, want 700", got)
	}
	if got := mode(t, path); got != 0o600 {
		t.Errorf("file mode = %o, want 600", got)
	}
}

func TestOpenLogFileAppends(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "k4s", "k4s.log")
	for _, line := range []string{"one\n", "two\n"} {
		f, err := openLogFile(path)
		if err != nil {
			t.Fatalf("openLogFile: %v", err)
		}
		if _, err := f.WriteString(line); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "one\ntwo\n" {
		t.Fatalf("log = %q, want both lines appended", got)
	}
}

func TestOpenLogFileRotation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		size        int
		oldRotated  bool
		wantRotated bool
	}{
		{name: "under limit stays", size: maxLogSize - 1, wantRotated: false},
		{name: "at limit rotates", size: maxLogSize, wantRotated: true},
		{name: "rotation replaces old .1", size: maxLogSize + 10, oldRotated: true, wantRotated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "k4s")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "k4s.log")
			body := bytes.Repeat([]byte("x"), tc.size)
			if err := os.WriteFile(path, body, 0o600); err != nil {
				t.Fatal(err)
			}
			if tc.oldRotated {
				if err := os.WriteFile(path+".1", []byte("stale"), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			f, err := openLogFile(path)
			if err != nil {
				t.Fatalf("openLogFile: %v", err)
			}
			t.Cleanup(func() { _ = f.Close() })

			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			rotated, rerr := os.ReadFile(path + ".1")
			if !tc.wantRotated {
				if info.Size() != int64(tc.size) {
					t.Errorf("log size = %d, want %d (untouched)", info.Size(), tc.size)
				}
				if rerr == nil {
					t.Errorf("unexpected %s.1", path)
				}
				return
			}
			if info.Size() != 0 {
				t.Errorf("log size = %d, want 0 after rotation", info.Size())
			}
			if rerr != nil {
				t.Fatalf("read rotated: %v", rerr)
			}
			if len(rotated) != tc.size {
				t.Errorf("rotated size = %d, want %d", len(rotated), tc.size)
			}
		})
	}
}

func TestOpenLogFileFailsUnderAFile(t *testing.T) {
	t.Parallel()

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := openLogFile(filepath.Join(blocker, "k4s", "k4s.log")); err == nil {
		t.Fatal("openLogFile under a regular file: want error")
	}
}
