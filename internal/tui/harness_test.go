package tui_test

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// The root tests look at the model only through messages in, messages out
// and the ANSI-stripped screen, so they survive the router refactor.

const (
	screenW = 160
	screenH = 40
)

// started returns the root model at screenW x screenH with its first view
// initialised, the way tea.Program starts it.
func started(t *testing.T, c *k8s.Client) tui.Model {
	t.Helper()
	m, _ := tuitest.Mount(t, tui.New(c, nil), screenW, screenH)
	return m
}

// colon returns the keys that run ":name" from the command bar.
func colon(name string) []tea.Msg {
	return []tea.Msg{tuitest.Press(":"), tuitest.Type(name), tuitest.Enter()}
}

// then joins key groups into one input list.
func then(groups ...[]tea.Msg) []tea.Msg {
	return slices.Concat(groups...)
}

func lines(m tui.Model) []string {
	return strings.Split(tuitest.Screen(m), "\n")
}

// header is the first screen line: "k4s · ctx: ... · ns: ... · view: ...".
func header(m tui.Model) string {
	return strings.TrimSpace(lines(m)[0])
}

// viewName is the active view's title as the header shows it.
func viewName(m tui.Model) string {
	h := header(m)
	_, after, ok := strings.Cut(h, "view: ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(after)
}

// footer is everything under the footer's top rule, trailing blanks
// trimmed: the hint (or error) line and the kubectl equivalent.
func footer(m tui.Model) string {
	ls := lines(m)
	for i := len(ls) - 1; i >= 0; i-- {
		if isRule(ls[i]) {
			return tuitest.TrimLines(strings.Join(ls[i+1:], "\n"))
		}
	}
	return ""
}

func isRule(line string) bool {
	s := strings.TrimSpace(line)
	return s != "" && strings.Trim(s, "─") == ""
}

func requireView(t *testing.T, m tui.Model, want string) {
	t.Helper()
	if got := viewName(m); got != want {
		t.Fatalf("view = %q, want %q\n%s", got, want, tuitest.Screen(m))
	}
}

func requireScreen(t *testing.T, m tui.Model, wants ...string) {
	t.Helper()
	screen := tuitest.Screen(m)
	for _, w := range wants {
		if !strings.Contains(screen, w) {
			t.Fatalf("screen lacks %q:\n%s", w, screen)
		}
	}
}

func refuteScreen(t *testing.T, m tui.Model, unwanted ...string) {
	t.Helper()
	screen := tuitest.Screen(m)
	for _, w := range unwanted {
		if strings.Contains(screen, w) {
			t.Fatalf("screen shows %q:\n%s", w, screen)
		}
	}
}

func hasQuit(msgs []tea.Msg) bool {
	return slices.ContainsFunc(msgs, tuitest.IsQuit)
}
