//go:build unix

package logsink

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A link planted where the log goes must not make k4s append to, or
// create, the file it points at.
func TestOpenLogFileRefusesASymlink(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		target string // "" leaves the link dangling
	}{
		{name: "link to an existing file", target: "keep\n"},
		{name: "dangling link", target: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			victim := filepath.Join(root, "victim")
			if tc.target != "" {
				if err := os.WriteFile(victim, []byte(tc.target), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			dir := filepath.Join(root, "k4s")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "k4s.log")
			if err := os.Symlink(victim, path); err != nil {
				t.Fatal(err)
			}

			if f, err := openLogFile(path); err == nil {
				_ = f.Close()
				t.Fatal("openLogFile followed a symlink")
			}
			got, err := os.ReadFile(victim)
			switch {
			case tc.target == "" && !errors.Is(err, fs.ErrNotExist):
				t.Errorf("the link's target was created (err %v)", err)
			case tc.target != "" && string(got) != tc.target:
				t.Errorf("target = %q, want %q untouched", got, tc.target)
			}
		})
	}
}
