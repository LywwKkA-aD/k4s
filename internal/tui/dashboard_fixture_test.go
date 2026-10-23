package tui_test

import (
	"context"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// answeredWith answers the dashboard's refreshes from the fake cluster c,
// each snapshot then passed through edit: the fake serves no Tables, so a
// test fills in by hand what the panels that read them would show.
func answeredWith(t *testing.T, c *k8s.Client, edit func(dash.Snapshot) dash.Snapshot) tui.Option {
	t.Helper()
	src, err := dash.NewSource(c)
	if err != nil {
		t.Fatal(err)
	}
	fetch := dash.NewFetcher(src).Fetch
	return tui.WithDashboardOptions(dashboard.WithFetch(func(ctx context.Context, p dash.Plan, prev dash.Snapshot) dash.Snapshot {
		return edit(fetch(ctx, p, prev))
	}))
}

// crashRow lists crash-0 in panel 3, as the pods Table would.
func crashRow(s dash.Snapshot) dash.Snapshot {
	crash := dash.PodRow{
		Key:      dash.ObjectKey{Kind: "pod", Namespace: tuitest.NamespaceDemo, Name: tuitest.PodCrash},
		Resolved: true, Status: "CrashLoopBackOff", Sev: dash.SevFailing,
		Cmd: dash.Cmd{Args: []string{"describe", "pod", tuitest.PodCrash, "-n", tuitest.NamespaceDemo}},
	}
	s.Pods = dash.PodsPanel{
		Avail: dash.Avail{State: dash.StateOK}, Cmd: dash.Cmd{Args: []string{"get", "pods", "-A"}},
		Total: 1, NotOK: 1, Phases: dash.PhaseCounts{Failing: 1}, Problems: []dash.PodRow{crash},
	}
	return s
}

// k3sPanel shows panel 6, as on a k3s cluster whose kube-system k4s reads.
func k3sPanel(s dash.Snapshot) dash.Snapshot {
	s.K3s = dash.K3sPanel{Shown: true, Cmd: dash.Cmd{Args: []string{"get", "deploy,ds", "-n", "kube-system"}}}
	return s
}
