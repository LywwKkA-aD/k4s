package dash

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// State is the outcome class of an endpoint's last request. Denials,
// absent APIs and timeouts are states the panels render, not errors.
type State int

// States. Stale, Throttled and Failed keep the endpoint's previous data.
const (
	StateUnknown      State = iota // never requested
	StateOK                        // answered
	StateForbidden                 // 403: shown as — plus a kubectl auth can-i probe, never as 0
	StateAbsent                    // 404, or the API group is not in discovery
	StateNotReady                  // 503: the API is registered but its backend is not ready
	StateStale                     // timeout or unreachable: last data kept and marked stale
	StateThrottled                 // 429: the whole poller backs off
	StateUnsupported               // 400: e.g. a field selector the server does not know
	StateUnauthorized              // 401: credentials expired
	StateFailed                    // any other error
)

var stateNames = []string{
	"unknown", "ok", "forbidden", "absent", "not ready", "stale", "throttled",
	"unsupported", "unauthorized", "failed",
}

func (s State) String() string {
	if s < 0 || int(s) >= len(stateNames) {
		return "unknown"
	}
	return stateNames[s]
}

// classify maps a request error to a State. It is a local stand-in for the
// M4.3 classifier and only needs the class, not the user-facing text.
func classify(err error) State {
	switch {
	case err == nil:
		return StateOK
	case errors.Is(err, errNoREST):
		return StateAbsent
	case apierrors.IsForbidden(err):
		return StateForbidden
	case apierrors.IsNotFound(err):
		return StateAbsent
	case apierrors.IsServiceUnavailable(err):
		return StateNotReady
	case apierrors.IsTooManyRequests(err):
		return StateThrottled
	case apierrors.IsBadRequest(err):
		return StateUnsupported
	case apierrors.IsUnauthorized(err):
		return StateUnauthorized
	case isTimeout(err):
		return StateStale
	default:
		return StateFailed
	}
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, syscall.ECONNREFUSED) ||
		apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) {
		return true
	}
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

// Usage is one endpoint's ledger entry: what its last request cost and how
// it ended. Plan reads it to pick the due set and stretch cadences.
type Usage struct {
	Fetched    bool          // at least one request went out in this context
	At         time.Duration // plan time of the last request
	State      State
	Namespace  string        // namespace of the last request; "" is cluster-wide
	Rows       int           // items or Table rows decoded
	Bytes      int           // body bytes decoded (protobuf size for typed lists)
	Wall       time.Duration // wall time of the last request
	StaleSince time.Duration // plan time the current run of stale results began
	Err        error         // the last error, for the status line; nil when OK
}

// Ledger holds every endpoint's Usage. It is an array, so copies are deep
// and a Snapshot never shares it.
type Ledger [numEndpoints]Usage

// Scope is the namespace the dashboard lists.
type Scope struct {
	Namespace string // "" lists all namespaces
	Context   string // the kubeconfig context's namespace: the fallback when -A is denied
	Fallback  bool   // Namespace came from Context because -A was forbidden
}
