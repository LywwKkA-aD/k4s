package logs

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func TestRolloutJoinsNewPodsAndEndsOldOnes(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	if got, want := r.m.Command().String(), "kubectl logs -f -l 'app=web,pod-template-hash in (h1)' --prefix --tail=100 --max-log-requests=2 -n shop"; got != want {
		t.Fatalf("footer = %q\nwant     %q", got, want)
	}

	// The rollout starts: a new ReplicaSet and its first pod.
	r.add(webReplicaSet("h2", "2"), webPod("web-h2-a", "h2", 600))
	r.advance(followInterval)
	opts := r.src.opened(t, "web-h2-a", 1)
	if want := (k8s.LogOptions{Container: "", Tail: 100, Since: created.Add(600 * time.Second), Timestamps: true}); opts != want {
		t.Errorf("the joined pod was asked for %+v, want %+v", opts, want)
	}
	if got, want := r.m.Command().String(), "kubectl logs -f -l 'app=web,pod-template-hash in (h2,h1)' --prefix --tail=100 --max-log-requests=3 -n shop"; got != want {
		t.Errorf("footer after the join = %q\nwant                 %q", got, want)
	}
	if want := "+ pod …-a joined (new ReplicaSet h2) · " + followNote; !slices.Contains(r.statuses(), want) {
		t.Errorf("statuses = %q, want %q", r.statuses(), want)
	}
	if got := r.calls(); got["list pods"] != 1 || got["list replicasets"] != 1 || got["get pods"] != 0 {
		t.Errorf("the rollout's re-resolve made %v", got)
	}

	// The old pods go: their streams end, the next list explains both.
	r.remove("web-h1-a", "web-h1-b")
	r.feed(r.end("web-h1-a", nil), r.end("web-h1-b", nil))
	r.advance(followInterval)
	r.shows("[web-h1-a] ○ ended: pod deleted", "[web-h1-b] ○ ended: pod deleted")
	if got := r.calls(); got["list pods"] != 2 || got["get pods"] != 0 {
		t.Errorf("explaining two ends made %v", got)
	}
	if got, want := r.m.Command().String(), "kubectl logs -f web-h2-a --tail=100 -n shop"; got != want {
		t.Errorf("footer after the rollout = %q, want %q", got, want)
	}
	if !strings.HasPrefix(r.m.Title(), "deployment · web · logs · web-h2-a") {
		t.Errorf("title = %q", r.m.Title())
	}
	// Lines keep their pod's prefix once several pods have streamed.
	r.feed(r.line("web-h2-a", created.Add(700*time.Second), "new version up"))
	r.send(tuitest.Press("t"))
	r.shows("[web-h2-a] new version up")
}

func TestPodsSharingTheLabelNeverJoin(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0)), k8s.LogKindDeployment, "web")
	job := webPod("web-cron-1", "", 60)
	job.Labels = map[string]string{"app": "web", "job-name": "web-cron-1"}
	yes := true
	job.OwnerReferences = []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: "web-cron-1", UID: "uid-job", Controller: &yes}}
	r.add(job)
	r.advance(followInterval)
	if r.src.count("web-cron-1") != 0 || len(r.statuses()) != 0 {
		t.Errorf("a CronJob's pod joined: %d streams, statuses %q", r.src.count("web-cron-1"), r.statuses())
	}
	if got := r.calls(); got["list replicasets"] != 0 {
		t.Errorf("a Job's pod listed ReplicaSets: %v", got)
	}
}

func TestAPendingPodWaitsThenJoins(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), pending(webPod("web-h1-b", "h1", 30))), k8s.LogKindDeployment, "web")
	r.shows("[web-h1-b] △ waiting: ContainerCreating")
	if r.src.count("web-h1-b") != 0 {
		t.Fatal("a pod still being created was streamed")
	}
	r.advance(followInterval)
	if strings.Count(r.screen(), "△ waiting") != 1 {
		t.Errorf("the wait is said again:\n%s", r.screen())
	}
	r.mutate("web-h1-b", func(p *corev1.Pod) {
		p.Status.Phase = corev1.PodRunning
		p.Status.ContainerStatuses[0].State = corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}}
	})
	r.advance(followInterval)
	if opts := r.src.opened(t, "web-h1-b", 1); !opts.Since.Equal(created.Add(30 * time.Second)) {
		t.Errorf("the started pod was asked for %+v, want its log from its creation", opts)
	}
}

func TestFollowCostsSixListsAMinute(t *testing.T) {
	t.Parallel()

	r := newRig(t, webObjects(webPod("web-h1-a", "h1", 0), webPod("web-h1-b", "h1", 0)), k8s.LogKindDeployment, "web")
	for range 6 {
		r.advance(followInterval)
	}
	if got := r.calls(); got["list pods"] != 6 || got["list replicasets"] != 0 || got["get pods"] != 0 || len(got) != 1 {
		t.Fatalf("a minute of following made %v, want 6 pod lists", got)
	}
	for _, a := range r.cs.Actions() {
		if l, ok := a.(interface{ GetListOptions() metav1.ListOptions }); ok && l.GetListOptions().ResourceVersion != "0" {
			t.Errorf("a list read past the watch cache: %+v", l.GetListOptions())
		}
	}
	if err := r.m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	r.cs.ClearActions()
	for range 6 {
		r.advance(followInterval)
	}
	r.advance(time.Minute)
	if got := r.calls(); len(got) != 0 {
		t.Errorf("a closed view made %v", got)
	}
}

func TestFollowSaysWhyInTheHelp(t *testing.T) {
	t.Parallel()

	follows := newRig(t, webObjects(webPod("web-h1-a", "h1", 0)), k8s.LogKindDeployment, "web")
	single := newRig(t, webObjects(webPod("web-h1-a", "h1", 0)), "", "", "web-h1-a")
	if !slices.Contains(tuitest.HelpKeys(follows.m), "new pods") || slices.Contains(tuitest.HelpKeys(single.m), "new pods") {
		t.Errorf("help: following %v, single %v", tuitest.HelpKeys(follows.m), tuitest.HelpKeys(single.m))
	}
}

func TestOnlyFiftyPodsStream(t *testing.T) {
	t.Parallel()

	target := k8s.LogTarget{Kind: k8s.LogKindService, Namespace: shop, Name: "web", Selector: "app=web"}
	objs := make([]*corev1.Pod, 0, 73)
	for i := range 73 {
		p := webPod(fmt.Sprintf("web-%02d", i), "h1", 0)
		p.UID = types.UID(p.Name)
		objs = append(objs, p)
		target.Pods = append(target.Pods, k8s.PodRef{Name: p.Name, Containers: []string{"app"}, State: k8s.PodStateOf(p)})
	}
	c, cs := k8stestClient(objs...)
	r := mount(t, c, cs, target.PodNames(), WithTarget(target))
	if want := "streaming 50 of 73 pods · narrow with :ns or a deployment"; !slices.Contains(r.statuses(), want) {
		t.Errorf("statuses = %q, want %q", r.statuses(), want)
	}
	if got := len(r.m.pods); got != maxStreams {
		t.Errorf("%d pods stream", got)
	}
	cmd := r.m.Command()
	if !strings.Contains(cmd.String(), "--max-log-requests=50 ") || !cmd.Approx || !strings.Contains(cmd.String(), "--tail=100 ") {
		t.Errorf("footer = %q (approx %v)", cmd.String(), cmd.Approx)
	}
	r.advance(followInterval)
	if r.src.count("web-72") != 0 {
		t.Error("a pod past the cap joined")
	}
}
