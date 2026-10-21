package namespaces_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

var errDown = errors.New("etcd down")

const downStatus = "list namespaces: etcd down"

func outageClient(t *testing.T, o *tuitest.Outage) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, o.Lists("namespaces", errDown))
}

func errorStatuses(msgs []tea.Msg) []views.StatusMsg {
	var out []views.StatusMsg
	for _, s := range tuitest.All[views.StatusMsg](msgs) {
		if s.Level == status.Error {
			out = append(out, s)
		}
	}
	return out
}

// TestFailedRefreshKeepsTheRowsAndSaysSo: the last good rows stay, a banner
// marks them stale, the error is a status once per text, and Enter still
// selects the visible row.
func TestFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o))
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // default

	o.Down()
	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	screen := tuitest.Screen(m)
	for _, want := range []string{"stale: etcd down · r retry", "<all>", tuitest.NamespaceDemo} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "namespaces unavailable") {
		t.Errorf("the error replaced the table:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}

	_, out = tuitest.Drive(t, m, tuitest.Enter())
	if !tuitest.Contains(out, views.NamespaceSelectedMsg{Namespace: tuitest.NamespaceDefault}) {
		t.Errorf("enter on the stale list asked:\n%s", tuitest.Describe(tuitest.Requests(out)))
	}

	m, out = tuitest.Drive(t, m, tuitest.Press("r"))
	if got := errorStatuses(out); len(got) != 0 {
		t.Errorf("the same error was said again: %v", got)
	}

	o.Up()
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if screen := tuitest.Screen(m); strings.Contains(screen, "stale:") {
		t.Errorf("banner stayed after the list recovered:\n%s", screen)
	}
}

// TestStaleListKeepsItsHeight: the banner takes a line off the table.
func TestStaleListKeepsItsHeight(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o))
	fresh := len(strings.Split(m.View(), "\n"))
	o.Down()
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got := len(strings.Split(m.View(), "\n")); got > fresh {
		t.Errorf("stale view is %d lines, fresh one %d", got, fresh)
	}
}

// TestFirstListFailingShowsTheErrorBody: with no rows to keep, the error
// stays the body, and it is a status too.
func TestFirstListFailingShowsTheErrorBody(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	o.Down()
	m, _ := fresh(outageClient(t, &o))
	m, out := tuitest.Mount(t, m, width, height)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "namespaces unavailable: "+downStatus) || strings.Contains(screen, "stale:") {
		t.Errorf("first failure should show the error body:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}
}

// TestRefreshWhileListingSaysSo: r with a list in flight answers instead
// of staying silent, and starts no second list.
func TestRefreshWhileListingSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := fresh(tuitest.NewFakeClient(t)) // the first load is in flight
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	want := views.StatusMsg{Level: status.Info, Text: "refresh already running"}
	if len(out) != 1 || out[0] != want {
		t.Errorf("r while listing emitted:\n%s\nwant only %+v", tuitest.Describe(out), want)
	}
}
