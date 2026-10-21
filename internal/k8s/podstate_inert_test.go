package k8s_test

import (
	"testing"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"

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

// controlFree reports s as valid UTF-8 with no C0 control but tab, no DEL
// and no C1 control.
func controlFree(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if (r < 0x20 && r != '\t') || (r >= 0x7f && r <= 0x9f) {
			return false
		}
	}
	return true
}

func TestPodStateOfIsInert(t *testing.T) {
	t.Parallel()

	fields := map[string]func(p *corev1.Pod, s string){
		"pod reason": func(p *corev1.Pod, s string) { p.Status.Reason = s },
		"waiting reason": func(p *corev1.Pod, s string) {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: s}}}}
		},
		"exit reason": func(p *corev1.Pod, s string) {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "app", State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: s, ExitCode: 1}}}}
		},
		"last exit reason": func(p *corev1.Pod, s string) {
			p.Status.ContainerStatuses = []corev1.ContainerStatus{{
				Name: "app", RestartCount: 1,
				State:                corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: s, ExitCode: 137}},
			}}
		},
		"scheduling reason": func(p *corev1.Pod, s string) {
			p.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: s}}
		},
	}
	for field, set := range fields {
		for name, payload := range hostile {
			t.Run(field+" "+name, func(t *testing.T) {
				t.Parallel()
				p := podWith(corev1.PodPending)
				p.Spec.Containers = []corev1.Container{{Name: "app"}}
				set(p, payload)
				for _, got := range stateStrings(k8s.PodStateOf(p)) {
					if !controlFree(got) {
						t.Errorf("PodStateOf kept %q", got)
					}
				}
			})
		}
	}
}

// stateStrings are the words of s a view may show.
func stateStrings(s k8s.PodState) []string {
	out := []string{s.Phase, s.Reason}
	for _, c := range s.Containers {
		out = append(out, c.Name, c.Waiting)
		for _, x := range []*k8s.ContainerExit{c.Exit, c.LastExit} {
			if x != nil {
				out = append(out, x.Reason)
			}
		}
	}
	return out
}
