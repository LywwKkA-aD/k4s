package tui

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	apiversion "k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/LywwKkA-aD/k4s/internal/cluster"
	"github.com/LywwKkA-aD/k4s/internal/k8s"
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

var versionResource = schema.GroupResource{Resource: "version"}

// versionReply is the reply to /version ask number attempt (0 is the
// first) under tok.
func versionReply(tok async.Token, attempt int, info *apiversion.Info, err error) versionMsg {
	return versionMsg{res: async.Result[*apiversion.Info]{Token: tok, Value: info, Err: err}, attempt: attempt}
}

// TestVersionReply: the answer names the distribution, a failure asks
// again later while retries are left, and a reply for the previous client
// changes nothing and asks nothing.
func TestVersionReply(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithContext("default"))
	m := New(c, nil, WithSession(startup.Session{Distro: cluster.Other}))
	current := m.ident.version
	refused := errors.New("connection refused")

	cases := []struct {
		name       string
		msg        versionMsg
		wantDistro cluster.Distro
		wantKnown  bool
		wantRetry  bool
	}{
		{name: "the answer names the distribution", msg: versionReply(current, 0, k3sVersion(), nil), wantDistro: cluster.K3s, wantKnown: true},
		{name: "a retry's answer", msg: versionReply(current, 2, k3sVersion(), nil), wantDistro: cluster.K3s, wantKnown: true},
		{name: "a reply for the previous client", msg: versionReply(async.New(), 0, k3sVersion(), nil), wantDistro: cluster.Other},
		{name: "a failure for the previous client", msg: versionReply(async.New(), 0, nil, refused), wantDistro: cluster.Other},
		{name: "a failed request asks again", msg: versionReply(current, 0, nil, refused), wantDistro: cluster.Other, wantRetry: true},
		{name: "a failed retry asks again", msg: versionReply(current, 2, nil, refused), wantDistro: cluster.Other, wantRetry: true},
		{name: "the last retry failed too", msg: versionReply(current, 3, nil, refused), wantDistro: cluster.Other},
		{name: "an empty answer asks again", msg: versionReply(current, 0, nil, nil), wantDistro: cluster.Other, wantRetry: true},
		{name: "a timeout asks again", msg: versionReply(current, 0, nil, context.DeadlineExceeded), wantDistro: cluster.Other, wantRetry: true},
		{name: "a 503 asks again", msg: versionReply(current, 0, nil, apierrors.NewServiceUnavailable("starting")), wantDistro: cluster.Other, wantRetry: true},
		{name: "RBAC's answer stands", msg: versionReply(current, 0, nil, apierrors.NewForbidden(versionResource, "", refused)), wantDistro: cluster.Other},
		{name: "a 401 stands", msg: versionReply(current, 0, nil, apierrors.NewUnauthorized("expired")), wantDistro: cluster.Other},
		{name: "a 404 stands", msg: versionReply(current, 0, nil, apierrors.NewNotFound(versionResource, "")), wantDistro: cluster.Other},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			next, cmd := m.Update(tc.msg)
			got := next.(Model)
			if got.startup.Distro != tc.wantDistro || (got.startup.ServerVersion != nil) != tc.wantKnown {
				t.Fatalf("distro %q, version known %v; want %q, %v", got.startup.Distro, got.startup.ServerVersion != nil, tc.wantDistro, tc.wantKnown)
			}
			if (cmd != nil) != tc.wantRetry {
				t.Fatalf("retry armed = %v, want %v", cmd != nil, tc.wantRetry)
			}
		})
	}
}

func TestVersionRetryBacksOff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempt int
		wait    time.Duration
		ok      bool
	}{
		{attempt: 0, wait: 15 * time.Second, ok: true},
		{attempt: 1, wait: 30 * time.Second, ok: true},
		{attempt: 2, wait: time.Minute, ok: true},
		{attempt: 3},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("after ask %d", tc.attempt), func(t *testing.T) {
			t.Parallel()
			if wait, ok := versionRetryAfter(tc.attempt); wait != tc.wait || ok != tc.ok {
				t.Fatalf("versionRetryAfter(%d) = %v, %v; want %v, %v", tc.attempt, wait, ok, tc.wait, tc.ok)
			}
		})
	}
}

// versionedFake answers /version from memory with k3sVersion.
func versionedFake() *k8s.Client {
	cs := fake.NewSimpleClientset()
	if fd, ok := cs.Discovery().(*fakediscovery.FakeDiscovery); ok {
		fd.FakedServerVersion = k3sVersion()
	}
	return &k8s.Client{Clientset: cs, Context: "default", CurrentContext: "default"}
}

// TestVersionRetryAsksAgain: when its wait is over, a retry asks the
// server again under the same client's token, unless the client changed
// or the version is known by then.
func TestVersionRetryAsksAgain(t *testing.T) {
	t.Parallel()

	unknown := New(versionedFake(), nil, WithSession(startup.Session{Distro: cluster.Other}))
	known := New(versionedFake(), nil, WithSession(startup.Session{Distro: cluster.K3s, ServerVersion: k3sVersion()}))
	cases := []struct {
		name string
		m    Model
		msg  versionRetryMsg
		asks bool
	}{
		{name: "the version is still unknown", m: unknown, msg: versionRetryMsg{token: unknown.ident.version, attempt: 1}, asks: true},
		{name: "the client changed", m: unknown, msg: versionRetryMsg{token: async.New(), attempt: 1}},
		{name: "the version is known", m: known, msg: versionRetryMsg{token: known.ident.version, attempt: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, cmd := tc.m.Update(tc.msg)
			if !tc.asks {
				if cmd != nil {
					t.Fatal("a retry asked the server after all")
				}
				return
			}
			reply, ok := tuitest.Find[versionMsg](tuitest.Collect(t, cmd))
			if !ok || reply.attempt != tc.msg.attempt || !tc.m.ident.version.Accept(reply.res.Token) {
				t.Fatalf("retry asked nothing, or not as ask %d of this client: %+v", tc.msg.attempt, reply)
			}
		})
	}
}

// TestVersionArrivesOnARetry drives a failed first ask through its retry
// to the header naming the distribution.
func TestVersionArrivesOnARetry(t *testing.T) {
	t.Parallel()

	m := New(versionedFake(), nil, WithSession(startup.Session{Distro: cluster.Other}))
	tok := m.ident.version
	next, armed := m.Update(versionReply(tok, 0, nil, errors.New("i/o timeout")))
	if armed == nil {
		t.Fatal("a failed ask armed no retry")
	}
	m, out := tuitest.Drive(t, next.(Model), versionRetryMsg{token: tok, attempt: 1})
	if _, ok := tuitest.Find[versionMsg](out); !ok {
		t.Fatalf("the retry asked nothing: %s", tuitest.Describe(out))
	}
	if m.startup.Distro != cluster.K3s || m.startup.ServerVersion == nil {
		t.Fatalf("distro %q, version %v after the retry", m.startup.Distro, m.startup.ServerVersion)
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
