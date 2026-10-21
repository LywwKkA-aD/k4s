package async

import (
	"context"
	"errors"
	"net"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// Outcome is how a poll ended, as far as the poller's cadence is concerned.
type Outcome int

const (
	// OK is a poll that succeeded. The slow-API backoff resets and a
	// throttle eases by one step.
	OK Outcome = iota
	// Failed is an error that says nothing about load, such as 403 or
	// 404. The cadence stays as it is.
	Failed
	// Slow is a timeout or an unreachable server. Consecutive ones double
	// the interval up to MaxBackoff; the next OK resets it.
	Slow
	// Throttled is the server asking for fewer requests (429, or another
	// status with Retry-After). It doubles the interval of the whole
	// poller, and each OK halves it back.
	Throttled
)

// Classifier maps a poll's error to its Outcome and the Retry-After the
// server asked for (zero when it named none). The dashboard can pass its
// own to report one Outcome for a snapshot of many endpoints.
type Classifier func(err error) (Outcome, time.Duration)

// Classify is the default Classifier. Timeouts are the context deadline,
// a network timeout, a 504 and a server-side timeout; a failed dial counts
// as Slow too, so a dead API is not asked every five seconds.
//
// client-go already sleeps through a 429 that carries Retry-After and
// retries it within the context deadline; such a poll then ends as Slow.
// A 429 that reaches the caller is Throttled.
func Classify(err error) (Outcome, time.Duration) {
	if err == nil {
		return OK, 0
	}
	retryAfter, asked := suggestedDelay(err)
	switch {
	case apierrors.IsTooManyRequests(err):
		return Throttled, retryAfter
	case timedOut(err), unreachable(err):
		return Slow, retryAfter
	case asked:
		return Throttled, retryAfter
	}
	return Failed, 0
}

func suggestedDelay(err error) (time.Duration, bool) {
	s, ok := apierrors.SuggestsClientDelay(err)
	if !ok || s <= 0 {
		return 0, false
	}
	return time.Duration(s) * time.Second, true
}

func timedOut(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func unreachable(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial"
}
