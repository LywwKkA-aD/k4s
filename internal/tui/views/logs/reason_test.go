package logs

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

func podIn(phase string, c k8s.ContainerState) k8s.PodState {
	if c.Name == "" {
		c.Name = "app"
	}
	return k8s.PodState{Phase: phase, Containers: []k8s.ContainerState{c}}
}

var started = time.Unix(1_700_000_000, 0)

func TestJudge(t *testing.T) {
	t.Parallel()

	evicted := podIn("Failed", k8s.ContainerState{})
	evicted.Reason = "Evicted"
	deleting := podIn("Running", k8s.ContainerState{Running: true, Started: started})
	deleting.Deleting = true
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "pods/log"}, "api-1", errors.New("no"))
	waitingToStart := apierrors.NewBadRequest(`container "app" in pod "api-1" is waiting to start: ContainerCreating`)

	cases := []struct {
		name     string
		found    bool
		state    k8s.PodState
		restarts int32
		err      error
		waiting  bool
		want     outcome
		note     string
	}{
		{name: "pod gone", want: outDeleted, note: "○ ended: pod deleted"},
		{name: "pod being deleted", found: true, state: deleting, want: outDeleted, note: "○ ended: pod deleted"},
		{
			name: "pod completed", found: true, state: podIn("Succeeded", k8s.ContainerState{Exit: &k8s.ContainerExit{Reason: "Completed"}}),
			want: outCompleted, note: "○ ended: pod completed (exit 0)",
		},
		{name: "pod evicted", found: true, state: evicted, want: outFailed, note: "○ ended: pod failed · Evicted"},
		{
			name: "pod failed", found: true, state: podIn("Failed", k8s.ContainerState{Exit: &k8s.ContainerExit{Reason: "Error", Code: 1}}),
			want: outFailed, note: "○ ended: pod failed · Error (exit 1)",
		},
		{name: "pod failed without a reason", found: true, state: podIn("Failed", k8s.ContainerState{}), want: outFailed, note: "○ ended: pod failed"},
		{
			name: "an init container completed", found: true, state: podIn("Running", k8s.ContainerState{Init: true, Exit: &k8s.ContainerExit{Reason: "Completed"}}),
			want: outCompleted, note: "○ ended: init container completed (exit 0)",
		},
		{
			name: "an init container failed", found: true, state: podIn("Pending", k8s.ContainerState{Init: true, Exit: &k8s.ContainerExit{Reason: "Error", Code: 1}}),
			want: outExited, note: "◇ container exited (Error, exit 1) · waiting for a restart",
		},
		{
			name: "a sidecar exited", found: true, state: podIn("Running", k8s.ContainerState{Init: true, Sidecar: true, Exit: &k8s.ContainerExit{Reason: "Completed"}}),
			want: outExited, note: "◇ container exited (Completed, exit 0) · waiting for a restart",
		},
		{
			name: "container restarted", found: true, restarts: 1,
			state: podIn("Running", k8s.ContainerState{Restarts: 2, Running: true, Started: started, LastExit: &k8s.ContainerExit{Reason: "OOMKilled", Code: 137}}),
			want:  outRestarted, note: "◇ container restarted (OOMKilled, exit 137) · reattached",
		},
		{
			name: "container restarted, cause unknown", found: true,
			state: podIn("Running", k8s.ContainerState{Restarts: 1, Running: true, Started: started}),
			want:  outRestarted, note: "◇ container restarted · reattached",
		},
		{
			name: "container exited, the pod will restart it", found: true,
			state: podIn("Running", k8s.ContainerState{Exit: &k8s.ContainerExit{Reason: "Error", Code: 1}}),
			want:  outExited, note: "◇ container exited (Error, exit 1) · waiting for a restart",
		},
		{
			name: "crash loop back-off", found: true, restarts: 3,
			state: podIn("Running", k8s.ContainerState{Restarts: 3, Waiting: "CrashLoopBackOff", LastExit: &k8s.ContainerExit{Reason: "Error", Code: 2}}),
			want:  outExited, note: "◇ container exited (Error, exit 2) · waiting for a restart",
		},
		{name: "stream dropped", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}), err: io.ErrUnexpectedEOF, want: outDropped, note: "◇ stream dropped · reconnected"},
		{name: "stream ended cleanly", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}), want: outDropped, note: "◇ stream dropped · reconnected"},
		{
			name: "status not caught up with a waiting container", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}),
			err: waitingToStart, want: outDropped, note: "◇ stream dropped · reconnected",
		},
		{name: "a refusal", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}), err: forbidden, want: outError, note: "○ ended: forbidden: pods/log · kubectl auth can-i --list"},
		{name: "a panic in k4s", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}), err: internalError{log: "the k4s log"}, want: outError, note: "○ ended: internal error (see the k4s log)"},
		{name: "the container is gone from the pod", found: true, state: k8s.PodState{Phase: "Running"}, err: io.EOF, want: outError, note: "○ ended: container app is not in the pod"},
		{name: "a pod being created", found: true, state: podIn("Pending", k8s.ContainerState{Waiting: "ContainerCreating"}), err: waitingToStart, want: outWaiting, note: "△ waiting: ContainerCreating"},
		{name: "a pod not placed yet", found: true, state: podIn("Pending", k8s.ContainerState{Waiting: "Unschedulable"}), waiting: true, want: outWaiting, note: "△ waiting: Unschedulable"},
		{name: "a waiting pod starts", found: true, state: podIn("Running", k8s.ContainerState{Running: true, Started: started}), waiting: true, want: outStarted},
		{name: "a waiting pod is deleted", waiting: true, want: outDeleted, note: "○ ended: pod deleted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := judge(end{found: tc.found, state: tc.state, container: "app", restarts: tc.restarts, err: tc.err, waiting: tc.waiting})
			if got.outcome != tc.want || got.note != tc.note {
				t.Fatalf("judge = %v %q, want %v %q", got.outcome, got.note, tc.want, tc.note)
			}
		})
	}
}

// Every notice reads the same in colour, stripped, as in mono, and each
// one is pinned in a golden file.
func TestNoticeLines(t *testing.T) {
	t.Parallel()

	notes := []string{
		"○ ended: pod deleted",
		"○ ended: pod completed (exit 0)",
		"○ ended: init container completed (exit 0)",
		"○ ended: pod failed · Evicted",
		"○ ended: pod failed · Error (exit 1)",
		"◇ container restarted (OOMKilled, exit 137) · reattached",
		"◇ container exited (Error, exit 1) · waiting for a restart",
		"◇ stream dropped · reconnected",
		"△ waiting: ContainerCreating",
		"○ end of the previous instance's log",
	}
	var golden strings.Builder
	for _, note := range notes {
		for _, multi := range []bool{false, true} {
			ln := logLine{pod: "api-7d9f-k7x2p", text: note, kind: lineNotice}
			coloured := formatRawLine(ln, multi, false, "#7D56F4")
			mono := "[api-7d9f-k7x2p] " + note
			if got := ansi.Strip(coloured); got != mono {
				t.Errorf("notice %q, multi %v: stripped %q, want %q", note, multi, got, mono)
			}
		}
		golden.WriteString(ansi.Strip(formatRawLine(logLine{pod: "api-7d9f-k7x2p", text: note, kind: lineNotice}, true, true, "")) + "\n")
	}
	tuitest.Golden(t, "notice_lines", golden.String())
}

func TestRetryDelays(t *testing.T) {
	t.Parallel()

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 30 * time.Second, 30 * time.Second}
	for i, w := range want {
		if got := retryDelay(i); got != w {
			t.Errorf("retryDelay(%d) = %v, want %v", i, got, w)
		}
	}
}
