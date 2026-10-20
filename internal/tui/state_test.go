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
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func isolatedState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	return dir
}

func saveForwards(t *testing.T, contexts ...string) {
	t.Helper()
	mgr, err := forwards.NewManager(nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	for i, ctx := range contexts {
		f := forwards.Forward{
			ID: forwards.NewID(), Context: ctx, Namespace: tuitest.NamespaceDemo,
			Kind: "pod", Name: tuitest.PodWeb1, LocalPort: uint16(19000 + i), RemotePort: 80,
		}
		if err := mgr.Register(f); err != nil {
			t.Fatalf("Register: %v", err)
		}
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
			requireScreen(t, m, "restore port-forwards", tc.want, "y / Enter revive all · n / Esc skip")
		})
	}
}

func TestRestorePromptKeys(t *testing.T) {
	cases := []struct {
		key    string
		closes bool
	}{
		{key: "y", closes: true},
		{key: "Y", closes: true},
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
			m, out := tuitest.Drive(t, started(t, nil), tuitest.Press(tc.key))
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

func TestRevivedForwardsShowInTheForwardsView(t *testing.T) {
	isolatedState(t)
	saveForwards(t, "", "")
	m, _ := tuitest.Drive(t, started(t, nil), tuitest.Press("y"))
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
	want := fmt.Sprintf("port-forward: local port %d already in use — pick another", addr.Port)
	if got := footer(m); !strings.Contains(got, want) {
		t.Errorf("footer = %q, want %q", got, want)
	}
	requireView(t, m, "dashboard")
	if fresh := started(t, nil); strings.Contains(tuitest.Screen(fresh), "restore port-forwards") {
		t.Error("a refused forward was saved")
	}
}

func TestBrokenStateDisablesForwards(t *testing.T) {
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
	m, _ = tuitest.Drive(t, m, views.ForwardRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodWeb1})
	if got := footer(m); !strings.Contains(got, "port-forward: state subsystem unavailable") {
		t.Errorf("footer = %q", got)
	}
	m, _ = tuitest.Drive(t, m, colon("pf")...)
	requireScreen(t, m, "port-forward subsystem unavailable")
}

func TestContextSwitchWithBrokenStateWarns(t *testing.T) {
	dir := isolatedState(t)
	m := started(t, tuitest.NewFakeClient(t))

	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "alpha"})
	if got := footer(m); !strings.Contains(got, "forwards: load state:") {
		t.Errorf("footer = %q", got)
	}
	if !strings.Contains(header(m), "ctx: alpha") {
		t.Errorf("the switch itself must still happen: %q", header(m))
	}
}
