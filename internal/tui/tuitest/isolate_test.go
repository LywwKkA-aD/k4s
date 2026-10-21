package tuitest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain runs the package under Isolate, which TestIsolate then checks
// from the inside.
func TestMain(m *testing.M) {
	os.Exit(tuitest.Isolate(m))
}

func TestIsolate(t *testing.T) {
	t.Parallel()

	state := os.Getenv("XDG_STATE_HOME")
	if !strings.Contains(state, "k4s-test-") {
		t.Errorf("XDG_STATE_HOME = %q, want a throwaway directory", state)
	}
	if _, err := os.Stat(state); err == nil {
		t.Errorf("%s exists: the restore popup would find saved forwards there", state)
	}
	cfg, err := os.ReadFile(os.Getenv("KUBECONFIG"))
	if err != nil {
		t.Fatalf("KUBECONFIG: %v", err)
	}
	if !strings.Contains(string(cfg), "current-context: \""+tuitest.FakeContext+"\"") {
		t.Errorf("kubeconfig lacks the %s context:\n%s", tuitest.FakeContext, cfg)
	}
}

func TestFooterIsWhatLiesUnderTheLastRule(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		view string
		want string
	}{
		{name: "hint and command", view: "body\n─────\nhints   \n≈ kubectl get pods\n\n", want: "hints\n≈ kubectl get pods"},
		{name: "the last rule wins", view: "a\n───\nb\n───\nc", want: "c"},
		{name: "no rule", view: "body only", want: ""},
		{name: "rule at the end", view: "body\n───", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tuitest.Footer(fixedView(tc.view)); got != tc.want {
				t.Errorf("Footer = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestColonRunsACommand(t *testing.T) {
	t.Parallel()

	keys := tuitest.Colon("pods")
	if len(keys) != 3 {
		t.Fatalf("Colon = %d messages, want 3", len(keys))
	}
	if n, _ := tuitest.KeyName(keys[0]); n != ":" {
		t.Errorf("first key = %q, want :", n)
	}
	if n, _ := tuitest.KeyName(keys[2]); n != "enter" {
		t.Errorf("last key = %q, want enter", n)
	}
}
