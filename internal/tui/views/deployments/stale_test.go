package deployments_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

var errDown = errors.New("etcd down")

const downStatus = "list deployments: etcd down"

func outageClient(t *testing.T, o *tuitest.Outage) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, tuitest.WithObjects(payObjects()...), o.Lists("deployments", errDown))
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

// TestFailedRefreshKeepsTheRowsAndSaysSo: the last good rows stay, a banner
// marks them stale, the error is a status once per text, and a row key
// still acts on the visible row.
func TestFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o), shop)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // pay-gateway

	o.Down()
	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	screen := tuitest.Screen(m)
	for _, want := range []string{"stale: etcd down · r retry", "pay-api", "payments"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "deployments unavailable") {
		t.Errorf("the error replaced the table:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}

	_, out = tuitest.Drive(t, m, tuitest.Enter())
	want := views.DescribeRequestMsg{Kind: "deployment", Namespace: shop, Name: "pay-gateway"}
	if !tuitest.Contains(out, want) {
		t.Errorf("enter on the stale list asked:\n%s", tuitest.Describe(tuitest.Requests(out)))
	}
	_, out = tuitest.Drive(t, m, tuitest.Press("l"))
	if _, ok := tuitest.Find[views.TailPromptRequestMsg](out); !ok {
		t.Errorf("l on the stale list did not resolve its pods:\n%s", tuitest.Describe(out))
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
	m, out := tuitest.Mount(t, deployments.New(outageClient(t, &o), shop), width, height)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "deployments unavailable: "+downStatus) || strings.Contains(screen, "stale:") {
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

	m := deployments.New(tuitest.NewFakeClient(t), shop) // the first load is in flight
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	want := views.StatusMsg{Level: status.Info, Text: "refresh already running"}
	if len(out) != 1 || out[0] != want {
		t.Errorf("r while listing emitted:\n%s\nwant only %+v", tuitest.Describe(out), want)
	}
}
