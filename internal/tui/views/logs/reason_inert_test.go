package logs

import (
	"io"
	"regexp"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
)

// hostile are status strings a node or a controller could write: an OSC 52
// clipboard write, a screen clear, a lone BEL, the C1 forms of CSI and OSC
// and a sequence left open.
var hostile = map[string]string{
	"osc 52":  "OOM\x1b]52;c;ZXZpbA==\x07Killed",
	"csi":     "Crash\x1b[2J\x1b[HLoop",
	"bel":     "Error\x07",
	"c1 csi":  "Evicted\u009b2J",
	"c1 osc":  "Pending\u009d0;pwned\u009c",
	"unended": "Unschedulable\x1b]0;title",
}

// sgr is the only escape the view draws itself: colour and weight.
var sgr = regexp.MustCompile("\x1b\\[[0-9;:]*m")

// drawnInert reports a rendered line with nothing but k4s's own colours.
func drawnInert(s string) bool {
	return inert(sgr.ReplaceAllString(s, ""))
}

// The view words a judged state safely even when the state was not made
// inert where it was read.
func TestJudgeIsInert(t *testing.T) {
	t.Parallel()

	running := k8s.ContainerState{Name: "app", Running: true, Started: started}
	cases := map[string]func(s string) end{
		"waiting reason": func(s string) end {
			return end{found: true, state: podIn("Pending", k8s.ContainerState{Waiting: s}), container: "app", waiting: true}
		},
		"pod reason": func(s string) end {
			st := podIn("Failed", k8s.ContainerState{})
			st.Reason = s
			return end{found: true, state: st, container: "app"}
		},
		"exit reason of a failed pod": func(s string) end {
			return end{found: true, state: podIn("Failed", k8s.ContainerState{Exit: &k8s.ContainerExit{Reason: s, Code: 1}}), container: "app"}
		},
		"exit reason": func(s string) end {
			return end{found: true, state: podIn("Running", k8s.ContainerState{Exit: &k8s.ContainerExit{Reason: s, Code: 1}}), container: "app"}
		},
		"last exit reason": func(s string) end {
			c := running
			c.Restarts, c.LastExit = 2, &k8s.ContainerExit{Reason: s, Code: 137}
			return end{found: true, state: podIn("Running", c), container: "app", restarts: 1}
		},
		"container name": func(s string) end {
			return end{found: true, state: podIn("Running", running), container: s, err: io.EOF}
		},
	}
	for field, build := range cases {
		for name, payload := range hostile {
			t.Run(field+" "+name, func(t *testing.T) {
				t.Parallel()
				v := judge(build(payload))
				if v.note == "" || !inert(v.note) {
					t.Fatalf("note %q", v.note)
				}
			})
		}
	}
}

// A pod's status reaches the backlog and the screen inert, whether it was
// read by the GET that explains an end or carried by the target the view
// was opened with.
func TestClusterStatusReachesTheScreenInert(t *testing.T) {
	t.Parallel()

	statuses := map[string]func(p *corev1.Pod, s string){
		"waiting reason": func(p *corev1.Pod, s string) {
			p.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: s}}
		},
		"pod reason": func(p *corev1.Pod, s string) { p.Status.Phase, p.Status.Reason = corev1.PodFailed, s },
		"exit reason": func(p *corev1.Pod, s string) {
			p.Status.Phase = corev1.PodFailed
			p.Status.ContainerStatuses[0].State = corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: s, ExitCode: 1}}
		},
	}
	for field, set := range statuses {
		for name, payload := range hostile {
			t.Run(field+" "+name, func(t *testing.T) {
				t.Parallel()
				r := onePod(t, webPod("web-h1-a", "h1", 0))
				r.mutate("web-h1-a", func(p *corev1.Pod) { set(p, payload) })
				r.feed(r.end("web-h1-a", nil))
				r.advance(explainDebounce)
				r.assertInert()
			})
		}
	}
	for name, payload := range hostile {
		t.Run("target state "+name, func(t *testing.T) {
			t.Parallel()
			p := webPod("web-h1-a", "h1", 0)
			p.UID = types.UID(p.Name)
			state := k8s.PodState{Phase: "Pending", Containers: []k8s.ContainerState{{Name: "app", Waiting: payload}}}
			target := k8s.LogTarget{
				Kind: k8s.LogKindService, Namespace: shop, Name: "web", Selector: "app=web",
				Pods: []k8s.PodRef{{Name: p.Name, Containers: []string{"app"}, State: state}},
			}
			c, cs := k8stestClient(p)
			r := mount(t, c, cs, target.PodNames(), WithTarget(target))
			r.assertInert()
		})
	}
}

// assertInert fails on a control byte in the backlog, or in what the view
// draws besides its own colours.
func (r *rig) assertInert() {
	r.t.Helper()
	if len(r.m.rawLines) == 0 {
		r.t.Fatal("nothing was said")
	}
	for _, ln := range r.m.rawLines {
		if !inert(ln.text) {
			r.t.Errorf("backlog holds %q", ln.text)
		}
	}
	for _, s := range r.m.rendered {
		if !drawnInert(s) {
			r.t.Errorf("rendered %q", s)
		}
	}
	if screen := r.m.View(); !drawnInert(strings.ReplaceAll(screen, "\n", "")) {
		r.t.Errorf("screen %q", screen)
	}
}
