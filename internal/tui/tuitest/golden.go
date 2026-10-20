package tuitest

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden is set with `go test ./internal/tui -run Footer -update`.
// Only test binaries that import tuitest know the flag, so name the package
// rather than passing it to ./... .
var updateGolden = flag.Bool("update", false, "rewrite testdata/*.golden files with the current output")

// goldenDir is relative to the package under test, as go test runs there.
const goldenDir = "testdata"

// Golden compares got with testdata/<name>.golden. With -update it writes
// got to the file instead. The file must exist otherwise: a missing golden
// fails the test rather than passing silently.
func Golden(t testing.TB, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(goldenDir, 0o750); err != nil {
			t.Fatalf("create %s: %v", goldenDir, err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (rerun with -update to create it): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s differs (rerun with -update to accept):\n%s", path, lineDiff(string(want), got))
	}
}

// lineDiff reports the first differing line with its neighbours, enough to
// read a footer or short screen mismatch without an external diff tool.
func lineDiff(want, got string) string {
	w := strings.Split(want, "\n")
	g := strings.Split(got, "\n")
	n := max(len(w), len(g))
	for i := range n {
		wl, gl := lineAt(w, i), lineAt(g, i)
		if wl != gl {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q\n--- want ---\n%s\n--- got ---\n%s", i+1, wl, gl, want, got)
		}
	}
	return "(no line differs; check trailing newlines)"
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<missing>"
}
