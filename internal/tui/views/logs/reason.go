package logs

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// outcome is what a pod's stream comes to once its state is known.
type outcome int

const (
	// outDeleted, outCompleted, outFailed and outError end the stream
	// for good.
	outDeleted outcome = iota + 1
	outCompleted
	outFailed
	outError
	// outRestarted reattaches to the container's new instance.
	outRestarted
	// outExited waits for the pod to restart its container.
	outExited
	// outWaiting waits for the container to start.
	outWaiting
	// outStarted attaches to a container that has started.
	outStarted
	// outDropped reattaches to the same instance after a backoff.
	outDropped
)

// end is what is known when a pod's stream is judged: its state from the
// nearest re-resolve, and how the stream ended.
type end struct {
	// found is false when the pod is gone.
	found bool
	state k8s.PodState
	// container is the container streamed; "" is the pod's default.
	container string
	// restarts is the container's restart count when its stream opened.
	restarts int32
	// err is how the stream ended, nil for the end of the log.
	err error
	// waiting is set for a pod that has not streamed yet.
	waiting bool
}

// verdict is the outcome, the notice that says it (empty for none) and
// the restart count of the container now.
type verdict struct {
	outcome  outcome
	note     string
	restarts int32
}

// Notices, after the pod's name. The glyphs tell them apart from a
// workload's own lines: ○ an end, ◇ a restart or reconnect, △ a wait.
const (
	noteDeleted   = "○ ended: pod deleted"
	noteDropped   = "◇ stream dropped · reconnected"
	notePrevious  = "○ end of the previous instance's log"
	noteEndedHead = "○ ended: "
)

// judge tells what became of a pod's stream, or of a pod waiting to
// stream, from its state now. A pod's own end wins over the stream's
// error: the stream of a deleted pod ends with one too. Each word of the
// state is made inert on its own as it goes into the note, wherever the
// state was read: one escape left open would swallow the rest.
func judge(e end) verdict {
	switch {
	case !e.found || e.state.Deleting:
		return verdict{outcome: outDeleted, note: noteDeleted}
	case e.state.Phase == "Succeeded":
		c, _ := e.state.Container(e.container)
		return verdict{outcome: outCompleted, note: "○ ended: pod completed (exit " + exitCode(c.Exit) + ")"}
	case e.state.Phase == "Failed":
		return verdict{outcome: outFailed, note: failedNote(e)}
	}
	c, ok := e.state.Container(e.container)
	switch {
	case !ok:
		return verdict{outcome: outError, note: noteEndedHead + missingContainer(e.container)}
	case !c.HasLog():
		return verdict{outcome: outWaiting, note: "△ waiting: " + orDefault(sanitize.Line(c.Waiting), "Pending"), restarts: c.Restarts}
	case e.waiting:
		return verdict{outcome: outStarted, restarts: c.Restarts}
	case e.restarts != restartsUnknown && c.Restarts > e.restarts:
		return verdict{outcome: outRestarted, note: "◇ container restarted" + exitDetail(c.LastExit) + " · reattached", restarts: c.Restarts}
	case c.Exit != nil:
		return verdict{outcome: outExited, note: "◇ container exited" + exitDetail(c.Exit) + " · waiting for a restart", restarts: c.Restarts}
	case c.Waiting != "" && c.LastExit != nil:
		return verdict{outcome: outExited, note: "◇ container exited" + exitDetail(c.LastExit) + " · waiting for a restart", restarts: c.Restarts}
	case hardError(e.err):
		return verdict{outcome: outError, note: noteEndedHead + errText(e.err), restarts: c.Restarts}
	}
	return verdict{outcome: outDropped, note: noteDropped, restarts: c.Restarts}
}

// failedNote names why a pod failed: its own reason (Evicted), else how
// its container ended.
func failedNote(e end) string {
	if reason := sanitize.Line(e.state.Reason); reason != "" {
		return "○ ended: pod failed · " + reason
	}
	c, _ := e.state.Container(e.container)
	if c.Exit == nil || c.Exit.Reason == "" {
		return "○ ended: pod failed"
	}
	return "○ ended: pod failed · " + sanitize.Line(c.Exit.Reason) + " (exit " + exitCode(c.Exit) + ")"
}

// exitDetail is " (OOMKilled, exit 137)", or empty when the exit is
// unknown.
func exitDetail(x *k8s.ContainerExit) string {
	if x == nil {
		return ""
	}
	if x.Reason == "" {
		return " (exit " + exitCode(x) + ")"
	}
	return " (" + sanitize.Line(x.Reason) + ", exit " + exitCode(x) + ")"
}

func exitCode(x *k8s.ContainerExit) string {
	if x == nil {
		return "0"
	}
	return strconv.Itoa(int(x.Code))
}

func missingContainer(name string) string {
	if name == "" {
		return "the pod has no containers"
	}
	return "container " + sanitize.Line(name) + " is not in the pod"
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// hardError reports a stream error that a reconnect would only repeat: a
// refusal from the API (4xx other than 408 and 429) or a panic in k4s.
// The API's "is waiting to start" is not one: the status the pod was
// judged by may not have caught up yet.
func hardError(err error) bool {
	if err == nil {
		return false
	}
	if errors.As(err, new(internalError)) {
		return true
	}
	var st apierrors.APIStatus
	if !errors.As(err, &st) {
		return false
	}
	code := int(st.Status().Code)
	switch {
	case code == http.StatusRequestTimeout || code == http.StatusTooManyRequests:
		return false
	case apierrors.IsBadRequest(err) && strings.Contains(st.Status().Message, "is waiting to start"):
		return false
	}
	return code >= 400 && code < 500
}

// errText is a stream error in words, inert: the API's own message
// without the request URL around it.
func errText(err error) string {
	return sanitize.Line(apiMessage(err))
}

// retrySteps are the waits before a dropped stream reconnects, the last
// one repeating.
var retrySteps = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 30 * time.Second}

// retryDelay is the wait before the reconnect that follows drops earlier
// drops in a row.
func retryDelay(drops int) time.Duration {
	return retrySteps[min(max(drops, 0), len(retrySteps)-1)]
}
