package logs_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/logs"
)

const (
	sample    = "alpha one\nbeta two\nalpha three\n"
	logsWidth = 100
)

func streaming(t *testing.T, body string, height int, pods ...string) logs.Model {
	t.Helper()
	c := tuitest.NewFakeClient(t, tuitest.WithLogs(body))
	m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, pods, 100, ""), logsWidth, height)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func requireTitle(t *testing.T, m logs.Model, want string) {
	t.Helper()
	if got := m.Title(); got != want {
		t.Fatalf("Title = %q, want %q", got, want)
	}
}

// fire delivers every timer of clock due within d, and what they bring.
func fire(t *testing.T, m logs.Model, clock *asynctest.Clock, d time.Duration) logs.Model {
	t.Helper()
	m, _ = tuitest.Drive(t, m, clock.Advance(d)...)
	return m
}

func TestLinesArriveAndAnEndedStreamSaysWhy(t *testing.T) {
	t.Parallel()

	c := tuitest.NewFakeClient(t, tuitest.WithLogs(sample))
	clock := asynctest.NewClock(time.Unix(1_700_000_000, 0))
	m, _ := tuitest.Mount(t, logs.New(c, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 100, "", logs.WithClock(clock)), logsWidth, 20)
	t.Cleanup(func() { _ = m.Close() })
	screen := tuitest.Screen(m)
	for _, w := range []string{"alpha one", "beta two", "alpha three"} {
		if !strings.Contains(screen, w) {
			t.Errorf("screen lacks %q:\n%s", w, screen)
		}
	}
	if strings.Contains(screen, "ended") {
		t.Errorf("the end is told before it is known:\n%s", screen)
	}
	requireTitle(t, m, "logs · web-1")

	if err := c.Clientset.CoreV1().Pods(tuitest.NamespaceDemo).Delete(context.Background(), tuitest.PodWeb1, metav1.DeleteOptions{}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	m = fire(t, m, clock, time.Second)
	if screen := tuitest.Screen(m); !strings.Contains(screen, "[web-1] ○ ended: pod deleted") {
		t.Errorf("the end does not say why:\n%s", screen)
	}
	if got := m.Command().String(); got != "kubectl logs -f web-1 --tail=100 -n demo" {
		t.Errorf("the ended pod's command = %q", got)
	}
}

func TestSearchFlow(t *testing.T) {
	t.Parallel()

	m := streaming(t, sample, 20, tuitest.PodWeb1)
	m, _ = tuitest.Drive(t, m, tuitest.Press("/"))
	if !m.CapturesKeys() {
		t.Fatal("the search prompt must capture keys")
	}
	m, _ = tuitest.Drive(t, m, tuitest.Type("alpha"), tuitest.Enter())
	if m.CapturesKeys() {
		t.Fatal("enter closes the search prompt")
	}
	requireTitle(t, m, "logs · web-1 · /alpha · 1/2 · paused")
	if screen := tuitest.Screen(m); !strings.Contains(screen, "/alpha · 1/2 (n/N step · c cancel)") {
		t.Errorf("status line:\n%s", screen)
	}

	steps := []struct {
		key  string
		want string
	}{
		{key: "n", want: "logs · web-1 · /alpha · 2/2 · paused"},
		{key: "n", want: "logs · web-1 · /alpha · 1/2 · paused"},
		{key: "N", want: "logs · web-1 · /alpha · 2/2 · paused"},
		{key: "c", want: "logs · web-1 · paused"},
		{key: "f", want: "logs · web-1"},
	}
	for _, s := range steps {
		m, _ = tuitest.Drive(t, m, tuitest.Press(s.key))
		requireTitle(t, m, s.want)
	}

	// With no search left, c clears the buffer.
	m, _ = tuitest.Drive(t, m, tuitest.Press("c"))
	if screen := tuitest.Screen(m); !strings.Contains(screen, "waiting for logs… (tail 100)") {
		t.Errorf("c without a search clears the lines:\n%s", screen)
	}
}

func TestSearchEdgeCases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		keys      []string
		text      string
		wantTitle string
		wantLine  string
	}{
		{name: "no match", keys: []string{"/"}, text: "zzz", wantTitle: "logs · web-1 · /zzz · no match", wantLine: "/zzz · no match (c cancel)"},
		{name: "esc abandons the prompt", keys: []string{"/"}, text: "alpha", wantTitle: "logs · web-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := streaming(t, sample, 20, tuitest.PodWeb1)
			m, _ = tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			m, _ = tuitest.Drive(t, m, tuitest.Type(tc.text))
			finish := tuitest.Enter()
			if tc.wantLine == "" {
				finish = tuitest.Esc()
			}
			m, _ = tuitest.Drive(t, m, finish)
			requireTitle(t, m, tc.wantTitle)
			if tc.wantLine != "" && !strings.Contains(tuitest.Screen(m), tc.wantLine) {
				t.Errorf("status line lacks %q:\n%s", tc.wantLine, tuitest.Screen(m))
			}
			// c while a search prompt is open cancels it.
			m, _ = tuitest.Drive(t, m, tuitest.Press("/"), tuitest.Press("c"))
			if !m.CapturesKeys() {
				t.Error("c inside the prompt is text, the prompt stays open")
			}
		})
	}
}

func TestDisplayToggles(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 300) + "\n"
	cases := []struct {
		name  string
		body  string
		pods  []string
		keys  []string
		want  string
		unset string
	}{
		{name: "wrap", body: sample, pods: []string{tuitest.PodWeb1}, keys: []string{"w"}, want: "[wrap on — press w to disable]"},
		{name: "wrap off again", body: sample, pods: []string{tuitest.PodWeb1}, keys: []string{"w", "w"}, unset: "[wrap on"},
		{name: "horizontal scroll", body: long, pods: []string{tuitest.PodWeb1}, keys: []string{"right"}, want: "[→ 10 cols]"},
		{name: "scroll back to the edge", body: long, pods: []string{tuitest.PodWeb1}, keys: []string{"right", "left", "left"}, unset: "[→"},
		{name: "wrap disables horizontal scroll", body: long, pods: []string{tuitest.PodWeb1}, keys: []string{"right", "w", "right"}, unset: "[→"},
		{name: "tags on several pods", body: sample, pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, keys: []string{"t"}, want: "[tags on — press t to compact]"},
		{name: "tags show the pod name", body: sample, pods: []string{tuitest.PodWeb1, tuitest.PodWeb2}, keys: []string{"t"}, want: "[web-2] alpha one"},
		{name: "tags do nothing for one pod", body: sample, pods: []string{tuitest.PodWeb1}, keys: []string{"t"}, unset: "[tags on"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := streaming(t, tc.body, 20, tc.pods...)
			m, _ = tuitest.Drive(t, m, tuitest.Keys(tc.keys...)...)
			screen := tuitest.Screen(m)
			if tc.want != "" && !strings.Contains(screen, tc.want) {
				t.Errorf("screen lacks %q:\n%s", tc.want, screen)
			}
			if tc.unset != "" && strings.Contains(screen, tc.unset) {
				t.Errorf("screen still shows %q:\n%s", tc.unset, screen)
			}
		})
	}
}

func TestScrollingUpPausesFollow(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	for i := range 60 {
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	m := streaming(t, b.String(), 10, tuitest.PodWeb1)
	requireTitle(t, m, "logs · web-1")

	m, _ = tuitest.Drive(t, m, tuitest.Press("up"))
	requireTitle(t, m, "logs · web-1 · paused")
	if !strings.Contains(tuitest.Screen(m), "[paused — press f to follow]") {
		t.Errorf("paused banner missing:\n%s", tuitest.Screen(m))
	}
	m, _ = tuitest.Drive(t, m, tuitest.Press("f"))
	requireTitle(t, m, "logs · web-1")
}

func TestStreamErrorsAndPlaceholders(t *testing.T) {
	t.Parallel()

	refused := tuitest.WithReactor("get", "pods", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetSubresource() != "log" {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "pods/log"}, tuitest.PodCrash, errors.New("not allowed"))
	})
	clock := asynctest.NewClock(time.Unix(1_700_000_000, 0))
	failing, _ := tuitest.Mount(t, logs.New(tuitest.NewFakeClient(t, refused), tuitest.NamespaceDemo, []string{tuitest.PodCrash}, 0, "", logs.WithClock(clock)), 100, 20)
	t.Cleanup(func() { _ = failing.Close() })
	failing = fire(t, failing, clock, time.Second)
	if screen := tuitest.Screen(failing); !strings.Contains(screen, `[crash-0] ○ ended: forbidden: pods/log · kubectl auth can-i --list`) {
		t.Errorf("stream error screen:\n%s", screen)
	}
	failing, _ = tuitest.Drive(t, failing, tuitest.Press("/"), tuitest.Type("forbidden"), tuitest.Enter())
	requireTitle(t, failing, "logs · crash-0 · /forbidden · 1/1 · paused")

	offline, out := tuitest.Init(t, logs.New(nil, tuitest.NamespaceDemo, []string{tuitest.PodWeb1}, 0, ""))
	if screen := tuitest.Screen(offline); !strings.Contains(screen, "no kubeconfig") {
		t.Errorf("offline screen: %q", screen)
	}
	if reqs := tuitest.Requests(out); len(reqs) != 0 {
		t.Errorf("an offline view asked for something:\n%s", tuitest.Describe(reqs))
	}
}

func TestTitleNamesTheContainer(t *testing.T) {
	t.Parallel()

	m := logs.New(nil, tuitest.NamespaceDemo, []string{tuitest.PodWeb1, tuitest.PodWeb2}, 0, tuitest.ContainerNginx)
	requireTitle(t, m, "logs · 2 pods / nginx")
}
