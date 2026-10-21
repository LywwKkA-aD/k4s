package tuitest_test

import (
	"os"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// TestMain runs the package under Isolate, which TestIsolate then checks
// from the inside.
func TestMain(m *testing.M) {
	os.Exit(tuitest.Isolate(m))
}

func TestIsolate(t *testing.T) {
	t.Parallel()

	if state := os.Getenv("XDG_STATE_HOME"); !strings.Contains(state, "k4s-test-") {
		t.Errorf("XDG_STATE_HOME = %q, want a throwaway directory", state)
	}
	got, err := k8s.ListContexts("")
	if err != nil {
		t.Fatalf("KUBECONFIG: %v", err)
	}
	if len(got) != 1 || got[0].Name != tuitest.FakeContext || !got[0].Current {
		t.Errorf("contexts = %+v, want only %s, current", got, tuitest.FakeContext)
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
