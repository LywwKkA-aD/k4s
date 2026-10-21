package deployments_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/deployments"
)

func TestSelectedIsTheRowUnderTheCursor(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name string
		keys []tea.Msg
		want views.ResourceRef
		ok   bool
	}{
		{name: "first row", want: views.ResourceRef{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentMulti}, ok: true},
		{name: "after down", keys: tuitest.Keys("down"), want: views.ResourceRef{Kind: "deployment", Namespace: demo, Name: tuitest.DeploymentWeb}, ok: true},
		{name: "filtered to nothing", keys: tuitest.FilterBy("zzz")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := asynctest.NewClock(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
			m, _ := tuitest.Mount(t, deployments.New(tuitest.NewFakeClient(t), demo, deployments.WithClock(clk)), 140, 30)
			m, _ = tuitest.Drive(t, m, tc.keys...)
			var sel views.Selection = m
			if got, ok := sel.Selected(); got != tc.want || ok != tc.ok {
				t.Errorf("Selected = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestNothingSelectedBeforeTheList(t *testing.T) {
	t.Parallel()

	for _, m := range []deployments.Model{deployments.New(nil, ""), deployments.New(tuitest.NewFakeClient(t), "")} {
		if got, ok := m.Selected(); ok {
			t.Errorf("Selected before any list = %+v", got)
		}
	}
}
