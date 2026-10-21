package tui_test

// These tests give the port-forward state its own directory with t.Setenv,
// which rules out t.Parallel: the variable is process-wide, and a saved
// forward would otherwise open the restore popup in every other test.

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/forwards"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func isolatedState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	return dir
}

// saveForwards leaves the state an earlier session would: one forward per
// context, saved without a server, "" for one saved before contexts were
// recorded.
func saveForwards(t *testing.T, contexts ...string) {
	t.Helper()
	saved := make([]forwards.Forward, 0, len(contexts))
	for i, ctx := range contexts {
		saved = append(saved, forwards.Forward{
			ID: forwards.NewID(), Context: ctx, Namespace: tuitest.NamespaceDemo,
			Kind: "pod", Name: tuitest.PodWeb1, LocalPort: uint16(19000 + i), RemotePort: 80,
		})
	}
	if err := (forwards.State{Forwards: saved}).Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// freePort returns a local port nothing listens on right now.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is %T", ln.Addr())
	}
	return addr.Port
}

func TestRestorePromptCountsSavedForwards(t *testing.T) {
	cases := []struct {
		name     string
		saved    []string
		withFake bool
		want     string
	}{
		{name: "one", saved: []string{""}, want: "Found 1 saved forward from a previous session."},
		{name: "several", saved: []string{"", ""}, want: "Found 2 saved forwards from a previous session."},
		{
			name: "only the current context and context-less ones count", withFake: true,
			saved: []string{tuitest.FakeContext, "", "elsewhere"},
			want:  "Found 2 saved forwards from a previous session.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			isolatedState(t)
			saveForwards(t, tc.saved...)
			m := started(t, nil)
			if tc.withFake {
				m = started(t, tuitest.NewFakeClient(t))
			}
			requireScreen(t, m, "restore port-forwards", tc.want, "Enter revive all · n / Esc skip")
		})
	}
}

// startedPastTheGrace starts the root on clk and lets the restore confirm's
// grace period run out, as a user reading the question would.
func startedPastTheGrace(t *testing.T, clk *clock) tui.Model {
	t.Helper()
	m := startedAt(t, nil, clk)
	clk.Advance(overlay.Grace)
	return m
}

func TestRestorePromptKeys(t *testing.T) {
	cases := []struct {
		key    string
		closes bool
	}{
		{key: "y"},
		{key: "Y"},
		{key: "enter", closes: true},
		{key: "n", closes: true},
		{key: "N", closes: true},
		{key: "q", closes: true},
		{key: "esc", closes: true},
		{key: "x"},
		{key: ":"},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			isolatedState(t)
			saveForwards(t, "")
			m, out := tuitest.Drive(t, startedPastTheGrace(t, newClock()), tuitest.Press(tc.key))
			if hasQuit(out) {
				t.Fatal("a restore key quit the app")
			}
			if tc.closes {
				refuteScreen(t, m, "restore port-forwards")
				requireView(t, m, "dashboard")
				return
			}
			requireScreen(t, m, "restore port-forwards")
		})
	}
}

// TestRestorePromptIgnoresTypeAhead: keys pressed before the question can
// have been read do not answer it.
func TestRestorePromptIgnoresTypeAhead(t *testing.T) {
	isolatedState(t)
	saveForwards(t, "")
	clk := newClock()
	m := startedAt(t, nil, clk)

	m, _ = tuitest.Drive(t, m, tuitest.Keys("y", "enter", "esc")...)
	requireScreen(t, m, "restore port-forwards")

	clk.Advance(overlay.Grace)
	m, _ = tuitest.Drive(t, m, tuitest.Press("n"))
	refuteScreen(t, m, "restore port-forwards")
}

func TestRevivedForwardsShowInTheForwardsView(t *testing.T) {
	isolatedState(t)
	saveForwards(t, "", "")
	m, _ := tuitest.Drive(t, startedPastTheGrace(t, newClock()), tuitest.Enter())
	m, _ = tuitest.Drive(t, m, colon("pf")...)
	requireView(t, m, "forwards")
	requireScreen(t, m, "19000 → 80", "19001 → 80")
}

func TestForwardPromptRegistersAndOpensForwards(t *testing.T) {
	isolatedState(t)
	port := freePort(t)
	req := views.ForwardRequestMsg{Kind: "service", Namespace: tuitest.NamespaceDemo, Name: tuitest.ServiceWeb, RemotePort: 80}

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req, clearInput(), tuitest.Type(fmt.Sprintf("%d:80", port)), tuitest.Enter())
	requireView(t, m, "forwards")
	requireScreen(t, m, "service", tuitest.ServiceWeb, fmt.Sprintf("%d → 80", port))
	if got := footer(m); !strings.Contains(got, fmt.Sprintf("kubectl port-forward -n demo service/web %d:80", port)) {
		t.Errorf("footer = %q", got)
	}

	// The intent is saved: the next session offers to revive it.
	next := started(t, tuitest.NewFakeClient(t))
	requireScreen(t, next, "Found 1 saved forward")
}

func TestForwardPromptRefusesABusyPort(t *testing.T) {
	isolatedState(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address is %T", ln.Addr())
	}

	req := views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}
	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req, tuitest.Type(fmt.Sprintf("%d:80", addr.Port)), tuitest.Enter())
	// The prompt stays open with the reason, so another port is one edit away.
	requireScreen(t, m, fmt.Sprintf("local port %d already in use — pick another", addr.Port), fmt.Sprintf("❯ %d:80", addr.Port))
	requireView(t, m, "dashboard")
	if fresh := started(t, nil); strings.Contains(tuitest.Screen(fresh), "restore port-forwards") {
		t.Error("a refused forward was saved")
	}
}

// unreadableState puts a directory where the state file belongs: k4s
// cannot read it, and must not move it.
func unreadableState(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "k4s", "portforwards.json"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestCorruptStateIsSetAside(t *testing.T) {
	dir := isolatedState(t)
	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := started(t, tuitest.NewFakeClient(t))
	refuteScreen(t, m, "restore port-forwards")
	m, _ = tuitest.Drive(t, m, colon("pf")...)
	if got := footer(m); !strings.Contains(got, "saved forwards were unreadable") {
		t.Errorf("footer = %q, want the notice about the corrupt file", got)
	}
	if aside, _ := filepath.Glob(path + ".corrupt-*"); len(aside) != 1 {
		t.Errorf("set aside = %v, want one .corrupt- file", aside)
	}
	m, _ = tuitest.Drive(t, m, views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	requireScreen(t, m, "format local:remote")
}

func TestBrokenStateDisablesForwards(t *testing.T) {
	unreadableState(t, isolatedState(t))

	m := started(t, tuitest.NewFakeClient(t))
	refuteScreen(t, m, "restore port-forwards")
	m, _ = tuitest.Drive(t, m, views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	if got := footer(m); !strings.Contains(got, "port-forward: state subsystem unavailable") {
		t.Errorf("footer = %q", got)
	}
	m, _ = tuitest.Drive(t, m, colon("pf")...)
	requireScreen(t, m, "port-forward subsystem unavailable")

	// A pair chosen before the state broke cannot be saved either.
	chosen := overlay.ForwardChosenMsg{Request: views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1}, Local: 9000, Remote: 80}
	m, _ = tuitest.Drive(t, m, chosen)
	if got := footer(m); !strings.Contains(got, "port-forward: state subsystem unavailable") {
		t.Errorf("footer = %q", got)
	}
	requireView(t, m, "forwards")
}

func TestForwardThatCannotBeSavedWarns(t *testing.T) {
	isolatedState(t)
	port := freePort(t)
	req := views.ForwardRequestMsg{Kind: "job", Namespace: tuitest.NamespaceDemo, Name: "nightly"}

	m, _ := tuitest.Drive(t, started(t, tuitest.NewFakeClient(t)), req, tuitest.Type(fmt.Sprintf("%d:80", port)), tuitest.Enter())
	if got := footer(m); !strings.Contains(got, `port-forward: unknown kind "job"`) {
		t.Errorf("footer = %q", got)
	}
	requireView(t, m, "dashboard")
	refuteScreen(t, m, "format local:remote")
}

func TestContextSwitchWithBrokenStateWarns(t *testing.T) {
	dir := isolatedState(t)
	m := started(t, tuitest.NewFakeClient(t))

	unreadableState(t, dir)
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "alpha"})
	if got := footer(m); !strings.Contains(got, "forwards: load state:") {
		t.Errorf("footer = %q", got)
	}
	if !strings.Contains(header(m), "ctx alpha") {
		t.Errorf("the switch itself must still happen: %q", header(m))
	}
}
