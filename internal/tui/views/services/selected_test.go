package services_test

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/services"
)

func TestSelectedIsTheRowUnderTheCursor(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		name      string
		namespace string
		keys      []tea.Msg
		want      views.ResourceRef
		ok        bool
	}{
		{name: "first row", namespace: demo, want: views.ResourceRef{Kind: "service", Namespace: demo, Name: tuitest.ServiceMulti}, ok: true},
		{name: "after down", namespace: demo, keys: tuitest.Keys("down"), want: views.ResourceRef{Kind: "service", Namespace: demo, Name: tuitest.ServiceWeb}, ok: true},
		{name: "all namespaces", keys: tuitest.Keys("down"), want: views.ResourceRef{Kind: "service", Namespace: demo, Name: tuitest.ServiceWeb}, ok: true},
		{name: "filtered to nothing", namespace: demo, keys: tuitest.FilterBy("zzz")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clk := asynctest.NewClock(time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC))
			m, _ := tuitest.Mount(t, services.New(tuitest.NewFakeClient(t), tc.namespace, services.WithClock(clk)), 140, 30)
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

	for _, m := range []services.Model{services.New(nil, ""), services.New(tuitest.NewFakeClient(t), "")} {
		if got, ok := m.Selected(); ok {
			t.Errorf("Selected before any list = %+v", got)
		}
	}
}
