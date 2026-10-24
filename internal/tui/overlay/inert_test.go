package overlay_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// Names from the cluster or the kubeconfig render inert in every prompt;
// what a prompt answers or previews keeps them raw, and a raw name kubectl
// would reject previews no command at all.
func TestHostileNamesRenderInert(t *testing.T) {
	t.Parallel()

	ns, pod, ctr := tuitest.Hostile("shop"), tuitest.Hostile("web-1"), tuitest.Hostile("app")
	target := views.ResourceRef{Kind: tuitest.Hostile("deployment"), Namespace: ns, Name: tuitest.Hostile("web")}
	refuse := func(uint16) error { return errors.New("port held by " + tuitest.Hostile("kubectl")) }
	cases := []struct {
		name string
		open func() overlay.Overlay
		keys []string
		want []string
	}{
		{
			name: "login names the context",
			open: func() overlay.Overlay {
				return overlay.NewLogin(tuitest.Hostile("eks-prod"), kubectl.New("version"), nil, nil, newClock().Now)
			},
			want: []string{"Context eks-prod needs a login"},
		},
		{
			name: "restore names the clusters",
			open: func() overlay.Overlay {
				return overlay.NewRestore(1, newClock().Now, tuitest.Hostile("prod")+" · "+tuitest.Hostile("10.0.0.5"))
			},
			want: []string{"prod · 10.0.0.5"},
		},
		{
			name: "container picker names the target and containers",
			open: func() overlay.Overlay {
				return overlay.NewContainer(views.ContainerPromptRequestMsg{
					Namespace: ns, Pods: []string{pod}, Containers: []string{ctr, "sidecar"}, Init: []string{ctr},
					NextKind: "logs", Target: target,
				})
			},
			want: []string{"deployment/web · ns shop", "1. init:app"},
		},
		{
			name: "container picker names one pod",
			open: func() overlay.Overlay {
				return overlay.NewContainer(views.ContainerPromptRequestMsg{
					Namespace: ns, Pods: []string{pod}, Containers: []string{ctr}, NextKind: "exec", Shell: "sh",
				})
			},
			want: []string{"pod/web-1 · ns shop", "1. app"},
		},
		{
			name: "tail prompt names the target",
			open: func() overlay.Overlay {
				return overlay.NewTail(views.TailPromptRequestMsg{Namespace: ns, Pods: []string{pod}, Container: ctr, Target: target})
			},
			want: []string{"deployment/web · ns shop"},
		},
		{
			name: "port prompt names the target and its error",
			open: func() overlay.Overlay {
				return overlay.NewForward(views.ForwardRequestMsg{Kind: tuitest.Hostile("pod"), Namespace: ns, Name: pod, RemotePort: 80}, refuse)
			},
			keys: []string{"enter"},
			want: []string{"pod/web-1 · shop", "port held by kubectl"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, _ := feed(t, tc.open(), keys(tc.keys...)...)
			if o == nil {
				t.Fatal("the overlay closed")
			}
			tuitest.AssertInert(t, o.View(80))
			for _, w := range tc.want {
				if !strings.Contains(screen(o), w) {
					t.Errorf("screen lacks the cleaned %q:\n%s", w, screen(o))
				}
			}
			if cmd, ok := overlay.CommandOf(o); ok && strings.ContainsAny(cmd.String(), "\x1b\a\u202e\u200b") {
				t.Errorf("command preview %q carries the raw name", cmd.String())
			}
		})
	}
}

// Picking a container answers the raw names it was asked for, and the
// footer previews no command for a pod name kubectl would reject.
func TestContainerPickerKeepsRawNames(t *testing.T) {
	t.Parallel()

	req := views.ContainerPromptRequestMsg{
		Namespace: tuitest.Hostile("shop"), Pods: []string{tuitest.Hostile("web-1")},
		Containers: []string{tuitest.Hostile("app")}, NextKind: "logs",
	}
	o := overlay.NewContainer(req)
	if cmd, ok := overlay.CommandOf(o); ok && !cmd.IsZero() {
		t.Errorf("preview = %q, want no command", cmd.String())
	}
	_, msgs := feed(t, o, keys("enter")...)
	want := overlay.ContainerChosenMsg{Request: req, Container: tuitest.Hostile("app")}
	got, ok := tuitest.Find[overlay.ContainerChosenMsg](msgs)
	if !ok || got.Container != want.Container || got.Request.Pods[0] != req.Pods[0] {
		t.Errorf("enter sent %v, want the raw %+v", msgs, want)
	}
}
