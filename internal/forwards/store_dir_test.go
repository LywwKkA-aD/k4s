package forwards

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// syncRecorder records the directories saveWith flushes.
type syncRecorder struct{ dirs []string }

func (r *syncRecorder) sync(dir string) error {
	r.dirs = append(r.dirs, dir)
	return nil
}

// TestSaveSyncsTheParentsOfWhatItCreated: a directory MkdirAll creates is
// an entry in its parent, so the parent is flushed too, or a power cut
// could lose the directory with the file in it.
func TestSaveSyncsTheParentsOfWhatItCreated(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "a", "b", "k4s")
	path := filepath.Join(dir, "portforwards.json")
	cases := []struct {
		name string
		want []string
	}{
		{name: "fresh nested path", want: []string{root, filepath.Join(root, "a"), filepath.Join(root, "a", "b"), dir}},
		{name: "the directory is there", want: []string{dir}},
	}
	// The second save finds what the first created, so they run in order.
	for _, tc := range cases {
		var rec syncRecorder
		if err := (State{}).saveWith(path, rec.sync); err != nil {
			t.Fatalf("%s: saveWith: %v", tc.name, err)
		}
		got := slices.Clone(rec.dirs)
		slices.Sort(got)
		want := slices.Clone(tc.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s: synced %q, want %q", tc.name, rec.dirs, tc.want)
		}
	}
}

func TestMissingDirs(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		dir  string
		want []string
	}{
		{name: "exists", dir: root},
		{name: "one level", dir: filepath.Join(root, "k4s"), want: []string{filepath.Join(root, "k4s")}},
		{name: "nested", dir: filepath.Join(root, "a", "k4s"), want: []string{filepath.Join(root, "a", "k4s"), filepath.Join(root, "a")}},
		{name: "under a file, where MkdirAll fails anyway", dir: filepath.Join(file, "k4s")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := missingDirs(tc.dir); !slices.Equal(got, tc.want) {
				t.Errorf("missingDirs(%q) = %q, want %q", tc.dir, got, tc.want)
			}
		})
	}
}

// TestSaveIntoAFreshNestedStateHome sets XDG_STATE_HOME, so it cannot run
// in parallel.
func TestSaveIntoAFreshNestedStateHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "x", "state")
	t.Setenv("XDG_STATE_HOME", home)

	want := State{Forwards: []Forward{{ID: "f1", Context: "dev", Namespace: "demo", Kind: "pod", Name: "web", LocalPort: 18080, RemotePort: 80}}}
	if err := want.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(filepath.Join(home, "k4s"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o700 {
		t.Errorf("state dir mode = %o, want 700", got)
	}
	got, err := Load()
	if err != nil || len(got.Forwards) != 1 || got.Forwards[0] != want.Forwards[0] {
		t.Errorf("Load = %+v, %v; want %+v", got, err, want)
	}
}
