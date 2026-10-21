package services_test

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

const downStatus = "list services: etcd down"

// outageClient is the pay fixture whose service list fails while o is down.
func outageClient(t *testing.T, o *tuitest.Outage) *k8s.Client {
	t.Helper()
	return payClient(t, o.Lists("services", errDown))
}

func errorStatuses(msgs []tea.Msg) []views.StatusMsg {
	var out []views.StatusMsg
	for _, s := range statuses(msgs) {
		if s.Level == status.Error {
			out = append(out, s)
		}
	}
	return out
}

// TestFailedRefreshKeepsTheRowsAndSaysSo: the rows of the last good list
// stay, a banner marks them stale, the error is a status once per text,
// and a row key still acts on the visible row.
func TestFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o), shop)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // pay-api

	o.Down()
	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	screen := tuitest.Screen(m)
	for _, want := range []string{"stale: etcd down · r retry", "pay-api", "payments"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "services unavailable") {
		t.Errorf("the error replaced the table:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}

	if got := describes(t, m); got != shop+"/pay-api" {
		t.Errorf("enter describes %q on the stale list, want pay-api", got)
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

// TestStaleListKeepsItsHeight: the banner takes a line off the table, so
// the view never renders taller than the body it was given.
func TestStaleListKeepsItsHeight(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o), shop)
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
	m, _ := clocked(outageClient(t, &o), shop)
	m, out := tuitest.Mount(t, m, width, height)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "services unavailable: "+downStatus) || strings.Contains(screen, "stale:") {
		t.Errorf("first failure should show the error body:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}
	_, out = tuitest.Drive(t, m, tuitest.Press("enter"))
	requireStatus(t, out, status.Warn, "services unavailable, press r")
}

// TestRefreshWhileListingSaysSo: r with a list in flight answers instead
// of staying silent.
func TestRefreshWhileListingSaysSo(t *testing.T) {
	t.Parallel()

	m, _ := clocked(payClient(t), shop) // New leaves the first load in flight
	_, cmd := press(t, m, "r")
	requireStatus(t, tuitest.Collect(t, cmd), status.Info, "refresh already running")
}
