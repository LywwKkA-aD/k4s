package kubectl_test

import (
	"bytes"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
)

// nasty covers what a pasted footer must survive: spaces, both quote kinds,
// expansions, globs, comments, history and brace expansion, and zsh's
// leading-= path expansion.
var nasty = []string{
	"",
	"plain",
	"app=web",
	"--tail=100",
	"app in (a,b)",
	"app=web,pod-template-hash in (7d9f8c6b5d,5c8d7b9f4)",
	"!app",
	"two words",
	"it's",
	`say "hi"`,
	"$HOME",
	"${KUBECONFIG}",
	"$(id)",
	"`id`",
	"*",
	"pod-?",
	"[ab]",
	"{a,b}",
	"~/kube",
	"#comment",
	"a;b",
	"a|b",
	"a&b",
	"a>b",
	"back\\slash",
	"=ls",
	"tab\there",
	"new\nline",
	"'",
	"''",
	"ünïcödé",
}

func TestQuote(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in, want string
	}{
		{"", "''"},
		{"pods", "pods"},
		{"app=web", "app=web"},
		{"--max-log-requests=15", "--max-log-requests=15"},
		{"/etc/rancher/k3s/k3s.yaml", "/etc/rancher/k3s/k3s.yaml"},
		{"user@cluster:prod,eu+1%", "user@cluster:prod,eu+1%"},
		{"app in (a,b)", "'app in (a,b)'"},
		{"two words", "'two words'"},
		{"it's", `'it'\''s'`},
		{"$HOME", "'$HOME'"},
		{"*", "'*'"},
		{"pod-?", "'pod-?'"},
		{"=ls", "'=ls'"},
		{"~/kube", "'~/kube'"},
		{"ünïcödé", "'ünïcödé'"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := kubectl.Quote(tt.in); got != tt.want {
				t.Errorf("Quote(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

// TestQuoteRoundTripsThroughShells hands every quoted word to real shells and
// checks each one reads back the exact original.
func TestQuoteRoundTripsThroughShells(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	shells := availableShells(t)
	for _, sh := range shells {
		t.Run(strings.Join(sh, " "), func(t *testing.T) {
			t.Parallel()
			words := make([]string, 0, len(nasty))
			for _, w := range nasty {
				words = append(words, kubectl.Quote(w))
			}
			got := shellSplit(t, sh, strings.Join(words, " "))
			if !slices.Equal(got, nasty) {
				t.Errorf("%v read back\n%q\nwant\n%q", sh, got, nasty)
			}
		})
	}
}

// TestStringRoundTripsToArgv is the footer contract: a pasted String()
// splits back into exactly Argv().
func TestStringRoundTripsToArgv(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("POSIX shells only")
	}
	cmd := kubectl.New(append([]string{"logs", "-f", "-l"}, nasty...)...).
		WithContext("it's prod").
		WithKubeconfig([]string{"/home/me/my configs/k3s.yaml"})
	for _, sh := range availableShells(t) {
		got := shellSplit(t, sh, cmd.String())
		if want := cmd.Argv(); !slices.Equal(got, want) {
			t.Errorf("%v split %s into\n%q\nwant\n%q", sh, cmd.String(), got, want)
		}
	}
}

// availableShells lists sh plus bash and zsh when installed; zsh runs with
// -f so a user's rc files cannot change parsing.
func availableShells(t *testing.T) [][]string {
	t.Helper()
	var out [][]string
	for _, sh := range [][]string{{"sh"}, {"bash", "--norc", "--noprofile"}, {"zsh", "-f"}} {
		if _, err := exec.LookPath(sh[0]); err == nil {
			out = append(out, sh)
		}
	}
	if len(out) == 0 {
		t.Skip("no POSIX shell on PATH")
	}
	return out
}

// shellSplit asks shell to word-split line and returns the words.
func shellSplit(t *testing.T, shell []string, line string) []string {
	t.Helper()
	script := `set -- ` + line + `; for w in "$@"; do printf '%s\000' "$w"; done`
	args := append(slices.Clone(shell[1:]), "-c", script)
	out, err := exec.Command(shell[0], args...).Output() //nolint:gosec // test drives a fixed shell
	if err != nil {
		t.Fatalf("%v -c %q: %v", shell, script, err)
	}
	parts := bytes.Split(out, []byte{0})
	words := make([]string, 0, len(parts))
	for _, p := range parts[:len(parts)-1] {
		words = append(words, string(p))
	}
	return words
}
