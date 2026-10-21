package tui

import (
	"errors"
	"testing"

	apiversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/startup"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func k3sVersion() *apiversion.Info { return &apiversion.Info{GitVersion: "v1.36.5+k3s1"} }

func TestFetchVersionOnlyWhereItCannotPrompt(t *testing.T) {
	t.Parallel()

	plain := *tuitest.NewFakeClient(t)
	plain.RestConfig = &rest.Config{Host: "https://10.0.0.5:6443"}
	exec := plain
	exec.RestConfig = &rest.Config{Host: plain.RestConfig.Host, ExecProvider: &clientcmdapi.ExecConfig{Command: "aws"}}

	cases := []struct {
		name string
		m    Model
		want bool
	}{
		{name: "certificate or token context", m: New(&plain, nil), want: true},
		{name: "exec plugin context", m: New(&exec, nil), want: false},
		{name: "version from the preflight", m: New(&plain, nil, WithSession(startup.Session{ServerVersion: k3sVersion()})), want: false},
		{name: "no client", m: New(nil, nil), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.m.fetchVersion() != nil; got != tc.want {
				t.Fatalf("fetches the version = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestOnVersion(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithContext("default"))
	m := New(c, nil, WithSession(startup.Session{Distro: cluster.Other}))
	current := m.ident.version

	cases := []struct {
		name       string
		msg        versionMsg
		wantDistro cluster.Distro
		wantKnown  bool
	}{
		{name: "the answer names the distribution", msg: versionMsg{res: async.Result[*apiversion.Info]{Token: current, Value: k3sVersion()}}, wantDistro: cluster.K3s, wantKnown: true},
		{name: "a reply for the previous client", msg: versionMsg{res: async.Result[*apiversion.Info]{Token: async.New(), Value: k3sVersion()}}, wantDistro: cluster.Other},
		{name: "a failed request", msg: versionMsg{res: async.Result[*apiversion.Info]{Token: current, Err: errors.New("connection refused")}}, wantDistro: cluster.Other},
		{name: "an empty answer", msg: versionMsg{res: async.Result[*apiversion.Info]{Token: current}}, wantDistro: cluster.Other},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := m.onVersion(tc.msg)
			if got.startup.Distro != tc.wantDistro || (got.startup.ServerVersion != nil) != tc.wantKnown {
				t.Fatalf("distro %q, version known %v; want %q, %v", got.startup.Distro, got.startup.ServerVersion != nil, tc.wantDistro, tc.wantKnown)
			}
		})
	}
}

// After :ctx the header asks the new cluster; the old client's token no
// longer fits.
func TestContextSwitchFetchesTheNewVersion(t *testing.T) {
	t.Parallel()

	m := New(tuitest.NewFakeClient(t), nil)
	before := m.ident.version
	next, out := tuitest.Drive(t, m, views.ContextSelectedMsg{Name: testContextBeta})
	if next.ident.version == before || next.ident.version.Accept(before) {
		t.Fatal("the switch kept the old client's version token")
	}
	if _, ok := tuitest.Find[versionMsg](out); !ok {
		t.Fatalf("no /version request after :ctx: %s", tuitest.Describe(out))
	}
}
