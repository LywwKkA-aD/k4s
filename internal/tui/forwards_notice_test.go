package tui_test

// These tests give the port-forward state its own directory with t.Setenv
// and do not run in parallel.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const corruptNotice = "forwards: saved forwards were unreadable ("

// corruptState leaves a state file in dir that does not parse; the next
// Manager sets it aside and has a notice to give.
func corruptState(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "k4s", "portforwards.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// corruptNotices are the statuses in msgs that report a corrupt state
// file, whoever said them.
func corruptNotices(msgs []tea.Msg) []views.StatusMsg {
	var out []views.StatusMsg
	for _, s := range tuitest.All[views.StatusMsg](msgs) {
		if strings.Contains(s.Text, "saved forwards were unreadable") {
			out = append(out, s)
		}
	}
	return out
}

func requireOneCorruptNotice(t *testing.T, msgs []tea.Msg) {
	t.Helper()
	got := corruptNotices(msgs)
	if len(got) != 1 || got[0].Level != status.Warn || !strings.HasPrefix(got[0].Text, corruptNotice) {
		t.Fatalf("corrupt state notices = %+v, want one warning starting %q", got, corruptNotice)
	}
}

func TestACorruptStateFileIsReportedAtStartupOnce(t *testing.T) {
	corruptState(t, isolatedState(t))

	m, out := tuitest.Mount(t, tui.New(tuitest.NewFakeClient(t), nil, instantFrames()), screenW, screenH)
	requireOneCorruptNotice(t, out)
	requireScreen(t, m, corruptNotice)

	m, out = tuitest.Drive(t, m, colon("pf")...)
	requireView(t, m, "forwards")
	if again := corruptNotices(out); len(again) != 0 {
		t.Errorf(":pf reported the corrupt file again: %+v", again)
	}
}

func TestACorruptStateFileIsReportedAfterAContextSwitch(t *testing.T) {
	dir := isolatedState(t)
	m := started(t, tuitest.NewFakeClient(t))
	corruptState(t, dir)

	m, _ = tuitest.Drive(t, m, views.ContextSelectedMsg{Name: "beta"})
	requireScreen(t, m, corruptNotice)

	_, out := tuitest.Drive(t, m, colon("pf")...)
	if again := corruptNotices(out); len(again) != 0 {
		t.Errorf(":pf reported the corrupt file again: %+v", again)
	}
}

// TestACorruptStateFileAndAFailedPreflightShowTogether: both are warnings
// at the start, and the status row holds one message, so they are one
// status that says both.
func TestACorruptStateFileAndAFailedPreflightShowTogether(t *testing.T) {
	corruptState(t, isolatedState(t))

	const preflight = "preflight: no answer from 10.0.0.5:6443 within 30s"
	c := tuitest.NewFakeClient(t)
	session := startup.Session{Client: c, PreflightErr: errors.New(preflight)}
	m, out := tuitest.Mount(t, tui.New(c, nil, tui.WithSession(session), instantFrames()), screenW, screenH)
	warnings := tuitest.All[views.StatusMsg](out)
	if len(warnings) != 1 || warnings[0].Level != status.Warn {
		t.Fatalf("statuses at the start = %+v, want one warning", warnings)
	}
	if got := warnings[0].Text; !strings.HasPrefix(got, preflight+" · "+corruptNotice) {
		t.Errorf("warning = %q, want the preflight, then the notice", got)
	}
	requireScreen(t, m, preflight, "saved forwards were unreadable")
}
