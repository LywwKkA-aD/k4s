package logs

import (
	"context"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestAFailedReresolveSaysSoOnce(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	var down atomic.Bool
	down.Store(true)
	r.cs.PrependReactor("list", "pods", func(ktesting.Action) (bool, runtime.Object, error) {
		if down.Load() {
			return true, nil, apierrors.NewServiceUnavailable("etcd is down")
		}
		return false, nil, nil
	})
	r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(explainDebounce)
	r.advance(followInterval)
	want := "logs of deployment web: cannot list its pods again: API unavailable (503)"
	if got := r.statuses(); len(got) != 1 || got[0] != want {
		t.Fatalf("statuses = %q, want %q once", got, want)
	}
	if strings.Contains(r.screen(), "◇") || r.src.count("web-h1-a") != 1 {
		t.Fatalf("an end was judged without the pods:\n%s", r.screen())
	}

	// Back up: the end left waiting is explained by the next round.
	down.Store(false)
	r.advance(followInterval)
	r.advance(retryDelay(0))
	r.shows("[web-h1-a] ◇ stream dropped · reconnected")
	r.src.opened(t, "web-h1-a", 2)
}

func TestAnEndDuringAReresolveIsExplainedByTheNext(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	target := r.m.target
	// A re-resolve is in flight when the stream ends.
	r.m.follow, _ = r.m.follow.Begin()
	r.m.explain.upTo = r.m.streams.ends
	r.feed(r.end("web-h1-a", io.ErrUnexpectedEOF))
	r.advance(explainDebounce)
	if got := r.calls(); len(got) != 0 {
		t.Fatalf("the end did not wait for the round in flight: %v", got)
	}
	// Its answer predates the end and leaves it; the next round explains it.
	r.send(async.Result[k8s.LogTarget]{Token: r.m.follow.Token(), Value: target})
	if r.m.streams.pods["web-h1-a"].phase != phaseEnded {
		t.Fatalf("an answer from before the end judged it: %+v", r.m.streams.pods["web-h1-a"])
	}
	if got := r.calls(); len(got) != 0 {
		t.Fatalf("the answer pulled the next round forward: %v", got)
	}
	r.advance(followInterval)
	if got := r.calls()["list pods"]; got != 1 {
		t.Fatalf("pod lists = %d, want the next round", got)
	}
	if r.m.streams.pods["web-h1-a"].phase != phaseRetry {
		t.Errorf("the end is still unexplained: %+v", r.m.streams.pods["web-h1-a"])
	}
}

func TestASecondPodTurnsPrefixesOn(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0)), k8s.LogKindDeployment, "web")
	r.feed(r.line("web-h1-a", stamped(1, 0), "alone"))
	if r.m.multi || slices.Contains(tuitest.HelpKeys(r.m), "t") {
		t.Fatal("one pod streams with a prefix")
	}
	r.add(webPod("web-h1-b", "h1", 60))
	r.advance(followInterval)
	if !r.m.multi || !slices.Contains(tuitest.HelpKeys(r.m), "t") {
		t.Fatal("a second pod left the lines without a prefix")
	}
	r.send(tuitest.Press("t"))
	r.shows("[web-h1-a] alone")
	if want := "+ pod …-b joined · " + followNote; !slices.Contains(r.statuses(), want) {
		t.Errorf("statuses = %q, want %q", r.statuses(), want)
	}
}

func TestContainerFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, view string
		pod        []string
		want       string
	}{
		{name: "the default container", pod: []string{"app"}},
		{name: "the pod has it", view: "proxy", pod: []string{"app", "proxy"}, want: "proxy"},
		{name: "the pod lacks it", view: "proxy", pod: []string{"worker"}, want: "worker"},
		{name: "nothing known of the pod", view: "proxy", want: "proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := Model{container: tc.view}
			if got := m.containerFor(k8s.PodRef{Containers: tc.pod}); got != tc.want {
				t.Errorf("containerFor = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShortNames(t *testing.T) {
	t.Parallel()

	for in, want := range map[string]string{"web-7d9f-k7x2p": "…-k7x2p", "standalone": "standalone"} {
		if got := shortPod(in); got != want {
			t.Errorf("shortPod(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"7d9f8c6b5d": "7d9f…", "h2": "h2"} {
		if got := shortHash(in); got != want {
			t.Errorf("shortHash(%q) = %q, want %q", in, got, want)
		}
	}
}

// migrateDeployment is the deployment web whose pods run the init
// container migrate before app.
func migrateDeployment() *appsv1.Deployment {
	d := webDeployment()
	d.Spec.Template.Spec.InitContainers = []corev1.Container{{Name: "migrate"}}
	return d
}

// withMigrate gives p the init container migrate, running.
func withMigrate(p *corev1.Pod) *corev1.Pod {
	p.Spec.InitContainers = []corev1.Container{{Name: "migrate"}}
	p.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name: "migrate", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}},
	}}
	return p
}

// followMigrate opens the logs of kind web in shop on the init container
// migrate, as the picker leaves them.
func followMigrate(t *testing.T, kind string, objs ...runtime.Object) *rig {
	t.Helper()
	c, cs := k8stest.NewFakeClient(objs...)
	target, err := c.ResolveLogTarget(context.Background(), kind, shop, "web")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	return mountContainer(t, c, cs, "migrate", target.PodNames(), WithTarget(target))
}

// said counts the statuses so far that contain text.
func (r *rig) said(text string) int {
	n := 0
	for _, s := range r.statuses() {
		if strings.Contains(s, text) {
			n++
		}
	}
	return n
}

// TestAPodWithoutThePickedContainerSaysItFellBack: a pod that lacks the
// container picked, one that joins or a service's pod other than the one
// the picker read, streams its first app container, and the view says so
// once.
func TestAPodWithoutThePickedContainerSaysItFellBack(t *testing.T) {
	t.Parallel()

	const want = "pod web-h1-b has no container migrate: streaming app"
	cases := []struct {
		name string
		open func(t *testing.T) *rig
	}{
		{
			name: "a pod that joins",
			open: func(t *testing.T) *rig {
				r := followMigrate(t, k8s.LogKindDeployment, migrateDeployment(), webReplicaSet("h1", "1"), withMigrate(webPod("web-h1-a", "h1", 0)))
				r.add(webPod("web-h1-b", "h1", 60))
				r.advance(followInterval)
				return r
			},
		},
		{
			name: "a service's other pod",
			open: func(t *testing.T) *rig {
				svc := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: shop},
					Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "web"}},
				}
				return followMigrate(t, k8s.LogKindService, svc, withMigrate(webPod("web-h1-a", "h1", 0)), webPod("web-h1-b", "h1", 0))
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := tc.open(t)
			if got := r.src.opened(t, "web-h1-b", 1).Container; got != "app" {
				t.Errorf("the pod without migrate streams %q, want app", got)
			}
			r.advance(followInterval)
			if n := r.said(want); n != 1 {
				t.Errorf("statuses = %q, want %q once", r.statuses(), want)
			}
		})
	}
}

func TestFallbackText(t *testing.T) {
	t.Parallel()

	b, c := fallback{pod: "web-b", container: "app"}, fallback{pod: "web-c", container: "app"}
	cases := []struct {
		name string
		view string
		fell []fallback
		want string
	}{
		{name: "none", view: "migrate"},
		{name: "one pod", view: "migrate", fell: []fallback{b}, want: "pod web-b has no container migrate: streaming app"},
		{name: "two pods", view: "migrate", fell: []fallback{b, c}, want: "pod web-b has no container migrate: streaming app (and 1 more pod)"},
		{name: "three pods", view: "migrate", fell: []fallback{b, c, c}, want: "pod web-b has no container migrate: streaming app (and 2 more pods)"},
		{
			name: "hostile names", view: "mig\x1b]0;x\x07rate", fell: []fallback{{pod: "web\x1b[2J-b", container: "a\x1b]52;c;eA==\x07pp"}},
			want: "pod web-b has no container migrate: streaming app",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := (Model{container: tc.view}).fallbackText(tc.fell); got != tc.want {
				t.Errorf("fallbackText = %q, want %q", got, tc.want)
			}
		})
	}
}
