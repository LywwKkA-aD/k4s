package describe_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/describe"
)

func TestDescribeRendersEachKind(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	cases := []struct {
		kind    describe.Kind
		name    string
		kubectl string
		want    []string
	}{
		{
			kind: describe.KindPod, name: tuitest.PodWeb1,
			kubectl: "kubectl describe pod web-1 -n demo",
			want:    []string{"Name:", tuitest.PodWeb1, "Namespace:", "Containers:", tuitest.ContainerNginx},
		},
		{
			kind: describe.KindDeployment, name: tuitest.DeploymentWeb,
			kubectl: "kubectl describe deployment web -n demo",
			want:    []string{"Name:", tuitest.DeploymentWeb, tuitest.ContainerNginx},
		},
		{
			kind: describe.KindService, name: tuitest.ServiceWeb,
			kubectl: "kubectl describe service web -n demo",
			want:    []string{"Name:", tuitest.ServiceWeb, "80"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			t.Parallel()
			m := describe.New(tuitest.NewFakeClient(t), tc.kind, demo, tc.name)
			if !strings.Contains(tuitest.Screen(m), "loading describe") {
				t.Errorf("before the fetch: %q", tuitest.Screen(m))
			}
			m, _ = tuitest.Mount(t, m, 120, 60)
			screen := tuitest.Screen(m)
			for _, w := range tc.want {
				if !strings.Contains(screen, w) {
					t.Errorf("screen lacks %q:\n%s", w, screen)
				}
			}
			if got := m.Command().String(); got != tc.kubectl {
				t.Errorf("Command = %q, want %q", got, tc.kubectl)
			}
			if got := m.Title(); got != string(tc.kind)+" · "+tc.name {
				t.Errorf("Title = %q for content that fits", got)
			}
		})
	}
}

func TestDescribeFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		m    describe.Model
		want string
	}{
		{name: "no kubeconfig", m: describe.New(nil, describe.KindPod, "demo", "x"), want: "no kubeconfig"},
		{name: "missing object", m: describe.New(tuitest.NewFakeClient(t), describe.KindPod, "demo", "nope"), want: "describe failed: get pod:"},
		{name: "unsupported kind", m: describe.New(tuitest.NewFakeClient(t), describe.Kind("secret"), "demo", "x"), want: "describe failed: unsupported kind: secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Mount(t, tc.m, 100, 20)
			if screen := tuitest.Screen(m); !strings.Contains(screen, tc.want) {
				t.Errorf("screen = %q, want %q", screen, tc.want)
			}
			if strings.Contains(m.Title(), "·  ") || strings.Contains(m.Title(), "more") {
				t.Errorf("failed describe shows a scroll hint: %q", m.Title())
			}
		})
	}
}

func TestDescribeScrollIndicator(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, describe.New(tuitest.NewFakeClient(t), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodMulti), 80, 5)
	if got := m.Title(); !strings.HasSuffix(got, "· top · ↓ more") {
		t.Errorf("long content at the top: Title = %q", got)
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("down"))
	if got := m.Title(); !strings.HasSuffix(got, "%") {
		t.Errorf("scrolled into the middle: Title = %q", got)
	}
	for range 20 {
		m, _ = tuitest.Drive(t, m, tuitest.Press("pgdown"))
	}
	if got := m.Title(); !strings.HasSuffix(got, "· end") {
		t.Errorf("scrolled to the bottom: Title = %q", got)
	}
}

func TestDescribeRefresh(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, describe.New(tuitest.NewFakeClient(t), describe.KindPod, tuitest.NamespaceDemo, tuitest.PodWeb1), 100, 40)
	_, out := tuitest.Step(t, m, tuitest.Press("r"))
	if len(out) != 1 {
		t.Errorf("r should refetch once:\n%s", tuitest.Describe(out))
	}

	offline := describe.New(nil, describe.KindPod, "demo", "x")
	if _, out := tuitest.Step(t, offline, tuitest.Press("r")); len(out) != 0 {
		t.Errorf("r without a client fetched:\n%s", tuitest.Describe(out))
	}
	if cmd := offline.Init(); cmd != nil {
		t.Error("Init without a client should not fetch")
	}
}

func TestDescribeStaticContract(t *testing.T) {
	t.Parallel()

	m := describe.New(nil, describe.KindService, "demo", "web")
	if m.CapturesKeys() || m.Close() != nil {
		t.Error("describe is read-only and owns nothing")
	}
	if n := len(m.Help()); n != 2 {
		t.Errorf("help bindings = %d, want scroll and refresh", n)
	}
}
