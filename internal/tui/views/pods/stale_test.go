package pods_test

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/pods"
)

var errDown = errors.New("etcd down")

const downStatus = "list pods: etcd down"

func outageClient(t *testing.T, o *tuitest.Outage) *k8s.Client {
	t.Helper()
	return tuitest.NewFakeClient(t, o.Lists("pods", errDown))
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
// marks them stale, the error is a status once per text, and a row key
// still acts on the visible row.
func TestFailedRefreshKeepsTheRowsAndSaysSo(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o), tuitest.NamespaceDemo)
	m, _ = tuitest.Drive(t, m, tuitest.Press("down")) // multi-0

	o.Down()
	m, out := tuitest.Drive(t, m, tuitest.Press("r"))
	screen := tuitest.Screen(m)
	for _, want := range []string{"stale: etcd down · r retry", tuitest.PodCrash, tuitest.PodWeb2} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "pods unavailable") {
		t.Errorf("the error replaced the table:\n%s", screen)
	}
	if got := errorStatuses(out); len(got) != 1 || got[0].Text != downStatus {
		t.Errorf("error statuses = %v, want one %q", got, downStatus)
	}

	_, out = tuitest.Drive(t, m, tuitest.Enter())
	if !tuitest.Contains(out, views.DescribeRequestMsg{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodMulti}) {
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
	m := mounted(t, outageClient(t, &o), tuitest.NamespaceDemo)
	fresh := len(strings.Split(m.View(), "\n"))
	o.Down()
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	if got := len(strings.Split(m.View(), "\n")); got > fresh {
		t.Errorf("stale view is %d lines, fresh one %d", got, fresh)
	}
}

// TestStaleListKeepsItsFilter: the filter still narrows the kept rows.
func TestStaleListKeepsItsFilter(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	m := mounted(t, outageClient(t, &o), tuitest.NamespaceDemo)
	o.Down()
	m, _ = tuitest.Drive(t, m, tuitest.Press("r"))
	m, _ = tuitest.Drive(t, m, tuitest.FilterBy("web")...)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "stale:") || !strings.Contains(screen, tuitest.PodWeb1) || strings.Contains(screen, tuitest.PodCrash) {
		t.Errorf("filtered stale list:\n%s", screen)
	}
}

// TestFirstListFailingShowsTheErrorBody: with no rows to keep, the error
// stays the body, and it is a status too.
func TestFirstListFailingShowsTheErrorBody(t *testing.T) {
	t.Parallel()

	var o tuitest.Outage
	o.Down()
	m, out := tuitest.Mount(t, pods.New(outageClient(t, &o), tuitest.NamespaceDemo), width, height)
	screen := tuitest.Screen(m)
	if !strings.Contains(screen, "pods unavailable: "+downStatus) || strings.Contains(screen, "stale:") {
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

	m := newView(tuitest.NewFakeClient(t), tuitest.NamespaceDemo) // the first load is in flight
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	want := views.StatusMsg{Level: status.Info, Text: "refresh already running"}
	if len(out) != 1 || out[0] != want {
		t.Errorf("r while listing emitted:\n%s\nwant only %+v", tuitest.Describe(out), want)
	}
}
