package viz_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

// update is set with `go test ./internal/tui/viz -update`.
var update = flag.Bool("update", false, "rewrite testdata/*.golden files with the current output")

// colour is a dark theme with the dashboard's token values, so the
// property tests see real foreground codes next to the mono attributes.
func colour() viz.Theme {
	fg := func(hex string) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex)) }
	return viz.Theme{
		Muted:   fg("#A8A8A8"),
		Accent:  fg("#A78BFA"),
		Accent2: fg("#3FC1C9"),
		OK:      fg("#04B575"),
		Warn:    fg("#F2A65A"),
		Err:     fg("#FF7B72").Bold(true),
	}
}

// golden compares got with testdata/<name>.golden; -update rewrites it.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *update {
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
		t.Errorf("%s differs (rerun with -update to accept)\n%s", path, firstDiff(string(want), got))
	}
}

func firstDiff(want, got string) string {
	w, g := strings.Split(want, "\n"), strings.Split(got, "\n")
	for i := range max(len(w), len(g)) {
		wl, gl := at(w, i), at(g, i)
		if wl != gl {
			return fmt.Sprintf("line %d:\n  want %q\n  got  %q", i+1, wl, gl)
		}
	}
	return "no line differs"
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<missing>"
}

// assertBlock fails unless s is exactly height lines of width columns.
func assertBlock(t *testing.T, s string, width, height int) {
	t.Helper()
	lines := strings.Split(s, "\n")
	if len(lines) != height {
		t.Fatalf("width %d: %d lines, want %d:\n%s", width, len(lines), height, ansi.Strip(s))
	}
	for i, l := range lines {
		if got := ansi.StringWidth(l); got != width {
			t.Errorf("width %d: line %d is %d columns: %q", width, i+1, got, ansi.Strip(l))
		}
	}
}

var sgr = regexp.MustCompile("\x1b\\[([0-9;:]*)m")

// colourCodes returns the SGR parameters in s that set a colour.
func colourCodes(s string) []int {
	var out []int
	for _, m := range sgr.FindAllStringSubmatch(s, -1) {
		for _, p := range strings.Split(m[1], ";") {
			head, _, _ := strings.Cut(p, ":")
			n, err := strconv.Atoi(head)
			if err != nil {
				continue
			}
			if isColourCode(n) {
				out = append(out, n)
			}
		}
	}
	return out
}

func isColourCode(n int) bool {
	switch {
	case n >= 30 && n <= 39, n >= 40 && n <= 49, n == 58, n == 59:
		return true
	case n >= 90 && n <= 97, n >= 100 && n <= 107:
		return true
	}
	return false
}

// plain is s as the goldens hold it: no escape codes.
func plain(s string) string { return ansi.Strip(s) }
