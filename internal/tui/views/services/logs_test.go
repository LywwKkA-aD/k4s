package services_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/status"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func serviceRef(ns, name string) views.ResourceRef {
	return views.ResourceRef{Kind: "service", Namespace: ns, Name: name}
}

func TestLogsOpensThePromptForTheService(t *testing.T) {
	t.Parallel()

	demo := tuitest.NamespaceDemo
	t.Run("one container goes to the tail prompt", func(t *testing.T) {
		t.Parallel()
		_, out := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), demo), on(tuitest.ServiceWeb, tuitest.Press("l"))...)
		got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
		if !ok {
			t.Fatalf("no tail prompt request:\n%s", tuitest.Describe(out))
		}
		want := views.TailPromptRequestMsg{
			Namespace: demo, Pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, Container: tuitest.ContainerNginx,
			Target: serviceRef(demo, tuitest.ServiceWeb), Logs: got.Logs,
		}
		if !equalTail(got, want) {
			t.Errorf("tail request = %+v\nwant %+v", got, want)
		}
		if got.Logs.Kind != k8s.LogKindService || got.Logs.Name != tuitest.ServiceWeb || got.Logs.Selector != "app=web" {
			t.Errorf("Logs = %+v, want the web service's target", got.Logs)
		}
		if len(statuses(out)) != 0 {
			t.Errorf("a clean resolve set a status:\n%s", tuitest.Describe(out))
		}
	})
	t.Run("several containers go to the picker", func(t *testing.T) {
		t.Parallel()
		_, out := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t), demo), on(tuitest.ServiceMulti, tuitest.Press("l"))...)
		got, ok := tuitest.Find[views.ContainerPromptRequestMsg](out)
		if !ok {
			t.Fatalf("no container prompt request:\n%s", tuitest.Describe(out))
		}
		if !slices.Equal(got.Pods, []string{tuitest.PodMulti}) || got.NextKind != "logs" ||
			!slices.Equal(got.Containers, []string{tuitest.ContainerApp, tuitest.ContainerSidecar}) ||
			got.Target != serviceRef(demo, tuitest.ServiceMulti) || got.Logs.Name != tuitest.ServiceMulti || got.Detached {
			t.Errorf("container request = %+v", got)
		}
	})
}

func equalTail(a, b views.TailPromptRequestMsg) bool {
	return a.Namespace == b.Namespace && slices.Equal(a.Pods, b.Pods) && a.Container == b.Container &&
		a.Target == b.Target && a.Detached == b.Detached
}

func finishedPod(name string, phase corev1.PodPhase, labels map[string]string) *corev1.Pod {
	p := tuitest.Pod(shop, name, labels, tuitest.ContainerNginx)
	p.Status.Phase = phase
	return p
}

func TestLogsSaysWhyItCannotOpen(t *testing.T) {
	t.Parallel()

	app := map[string]string{"app": "done"}
	boom := tuitest.WithReactor("get", "services", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("boom")
	})
	cases := []struct {
		name    string
		objs    []runtime.Object
		opts    []tuitest.FakeOption
		service string
		level   status.Level
		text    string
	}{
		{
			name:    "the kubernetes service has no selector",
			objs:    []runtime.Object{tuitest.Service(shop, "kubernetes", 443, nil)},
			service: "kubernetes", level: status.Warn, text: "service kubernetes has no selector",
		},
		{
			name:    "no pod matches",
			objs:    []runtime.Object{tuitest.Service(shop, "lonely", 80, map[string]string{"app": "ghost"})},
			service: "lonely", level: status.Warn, text: "no pods back service lonely",
		},
		{
			name: "only finished pods match",
			objs: []runtime.Object{
				tuitest.Service(shop, "done", 80, app),
				finishedPod("done-1", corev1.PodSucceeded, app),
			},
			service: "done", level: status.Warn,
			text: "no pods back service done; 1 completed pod skipped (kubectl logs -l would include it)",
		},
		{
			name:    "the lookup fails",
			objs:    []runtime.Object{tuitest.Service(shop, "web", 80, map[string]string{"app": "web"})},
			opts:    []tuitest.FakeOption{boom},
			service: "web", level: status.Error, text: "logs for service web: get service: boom",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			opts := append([]tuitest.FakeOption{tuitest.WithObjects(tc.objs...)}, tc.opts...)
			_, out := tuitest.Drive(t, mounted(t, tuitest.NewFakeClient(t, opts...), shop), on(tc.service, tuitest.Press("l"))...)
			requireStatus(t, out, tc.level, tc.text)
			if n := len(tuitest.Requests(out)) - len(statuses(out)); n != 0 {
				t.Errorf("emitted %d requests besides the status:\n%s", n, tuitest.Describe(out))
			}
		})
	}
}

func TestLogsOnAServiceThatIsGoneSaysSo(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t)
	m := mounted(t, c, tuitest.NamespaceDemo)
	deleteService(t, c, tuitest.NamespaceDemo, tuitest.ServiceWeb)
	_, out := tuitest.Drive(t, m, on(tuitest.ServiceWeb, tuitest.Press("l"))...)
	requireStatus(t, out, status.Warn, "service web is gone, press r")
}

func TestLogsNotesSkippedPods(t *testing.T) {
	t.Parallel()

	app := map[string]string{"app": "batch"}
	c := tuitest.NewFakeClient(t, tuitest.WithObjects(
		tuitest.Service(shop, "batch", 80, app),
		tuitest.Pod(shop, "batch-1", app, tuitest.ContainerNginx),
		finishedPod("batch-2", corev1.PodFailed, app),
	))
	_, out := tuitest.Drive(t, mounted(t, c, shop), on("batch", tuitest.Press("l"))...)
	requireStatus(t, out, status.Info, "≈ 1 failed pod skipped (kubectl logs -l would include it)")
	got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
	if !ok || !slices.Equal(got.Pods, []string{"batch-1"}) || len(got.Logs.Skipped) != 1 {
		t.Fatalf("want a tail prompt for batch-1 only:\n%s", tuitest.Describe(out))
	}
}

// TestLogsLastRequestWins: l on one row, then l on another before the
// first answer: only the second opens a prompt.
func TestLogsLastRequestWins(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	m, first := press(t, m, "l") // multi, the first row
	m, second := press(t, m, "down", "l")
	m, out := tuitest.Drive(t, m, tuitest.Collect(t, second)...)
	got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
	if !ok || got.Target.Name != tuitest.ServiceWeb || got.Detached {
		t.Fatalf("want the web prompt, attached:\n%s", tuitest.Describe(out))
	}
	_, late := tuitest.Drive(t, m, tuitest.Collect(t, first)...)
	if reqs := tuitest.Requests(late); len(reqs) != 0 {
		t.Fatalf("the superseded reply was taken:\n%s", tuitest.Describe(reqs))
	}
}

func TestLogsRepeatOnTheSameRowIsIgnored(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	m, first := press(t, m, "l")
	m, again := press(t, m, "l")
	if msgs := tuitest.Collect(t, again); len(msgs) != 0 {
		t.Fatalf("a repeat l resolved again:\n%s", tuitest.Describe(msgs))
	}
	_, out := tuitest.Drive(t, m, tuitest.Collect(t, first)...)
	if n := len(tuitest.All[views.ContainerPromptRequestMsg](out)); n != 1 {
		t.Fatalf("want one prompt, got %d:\n%s", n, tuitest.Describe(out))
	}
}

// TestLogsSlowResolveSaysSo: after 150 ms without an answer the status
// line says what is being resolved; an answer in time says nothing.
func TestLogsSlowResolveSaysSo(t *testing.T) {
	t.Parallel()

	const text = "resolving logs for service demo/multi…"
	cases := []struct {
		name     string
		wait     time.Duration
		answered bool
		want     bool
	}{
		{name: "still resolving at 150 ms", wait: 150 * time.Millisecond, want: true},
		{name: "not yet 150 ms", wait: 149 * time.Millisecond},
		{name: "answered in time", wait: 150 * time.Millisecond, answered: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, clk := clocked(tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
			m, _ = tuitest.Mount(t, m, width, height)
			m, cmd := press(t, m, "l")
			reply := tuitest.Collect(t, cmd)
			if tc.answered {
				m, _ = tuitest.Drive(t, m, reply...)
			}
			_, out := tuitest.Drive(t, m, clk.Advance(tc.wait)...)
			got := slices.Contains(statuses(out), views.StatusMsg{Level: status.Info, Text: text})
			if got != tc.want {
				t.Fatalf("status %q shown = %v, want %v:\n%s", text, got, tc.want, tuitest.Describe(out))
			}
		})
	}
}

// TestLogsReplyForALeftRowIsDetached: the cursor moved on while the logs
// resolved; the prompt still opens, marked.
func TestLogsReplyForALeftRowIsDetached(t *testing.T) {
	t.Parallel()

	m := mounted(t, tuitest.NewFakeClient(t), tuitest.NamespaceDemo)
	m, held := press(t, m, "down", "l") // web
	m, _ = press(t, m, "up")
	_, out := tuitest.Drive(t, m, tuitest.Collect(t, held)...)
	got, ok := tuitest.Find[views.TailPromptRequestMsg](out)
	if !ok || got.Target.Name != tuitest.ServiceWeb || !got.Detached {
		t.Fatalf("want a detached web prompt:\n%s", tuitest.Describe(out))
	}
}
