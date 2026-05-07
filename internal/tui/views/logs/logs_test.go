package logs

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestFormatLineSinglePodNoPrefix(t *testing.T) {
	t.Parallel()
	got := formatLine("nginx-7d-aaa", "hello world", false)
	if got != "hello world" {
		t.Errorf("single-pod line should pass through unchanged, got %q", got)
	}
}

func TestFormatLineMultiPodHasPrefix(t *testing.T) {
	t.Parallel()
	got := formatLine("nginx-7d-aaa", "hello world", true)
	if !strings.Contains(got, "nginx-7d-aaa") {
		t.Errorf("multi-pod line missing pod tag: %q", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("multi-pod line missing message: %q", got)
	}
}

func TestPodColorIsDeterministic(t *testing.T) {
	t.Parallel()
	a1 := podColor("nginx-aaa")
	a2 := podColor("nginx-aaa")
	if a1 != a2 {
		t.Errorf("podColor not deterministic: %v vs %v", a1, a2)
	}
}

func TestPodColorBelongsToPalette(t *testing.T) {
	t.Parallel()
	got := podColor("anything")
	palette := []lipgloss.Color{
		"#7D56F4", "#04B575", "#F2A65A", "#FF6B9D", "#56B4D4",
		"#E5C07B", "#98C379", "#C678DD", "#56B6C2", "#E06C75",
	}
	for _, c := range palette {
		if got == c {
			return
		}
	}
	t.Errorf("podColor returned %v, not in palette", got)
}

func TestAppendLineEnforcesBufferCap(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", nil)
	for i := 0; i < maxBufferedLines+50; i++ {
		m.appendLine("line")
	}
	if got := len(m.lines); got != maxBufferedLines {
		t.Errorf("buffer cap not enforced: got %d, want %d", got, maxBufferedLines)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"})
	if err := m.Close(); err != nil {
		t.Errorf("first Close() error: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Errorf("second Close() error: %v", err)
	}
}

func TestNewWithNoClientClosesChannelImmediately(t *testing.T) {
	t.Parallel()
	m := New(nil, "ns", []string{"foo"})
	// Channel must close so Init's wait Cmd returns instead of blocking.
	if _, ok := <-m.events; ok {
		t.Errorf("expected closed events channel when client is nil")
	}
}

func TestKubectlEquivalentSingleVsMulti(t *testing.T) {
	t.Parallel()

	single := New(nil, "demo", []string{"alpha"})
	if !strings.Contains(single.KubectlEquivalent(), "alpha") {
		t.Errorf("single-pod equivalent missing pod name: %q", single.KubectlEquivalent())
	}
	if strings.Contains(single.KubectlEquivalent(), "{") {
		t.Errorf("single-pod equivalent should not be brace-form: %q", single.KubectlEquivalent())
	}

	multi := New(nil, "demo", []string{"alpha", "beta"})
	if !strings.Contains(multi.KubectlEquivalent(), "alpha,beta") {
		t.Errorf("multi-pod equivalent missing pod list: %q", multi.KubectlEquivalent())
	}
}
