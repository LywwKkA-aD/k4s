package dash

import (
	"errors"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
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

// keepsData reports whether a state leaves the previous payload in place.
func (s State) keepsData() bool {
	return s == StateStale || s == StateThrottled || s == StateFailed || s == StateUnauthorized
}

// classify maps a request error to a State through the shared classifier
// (k8s.Classify), so a failed credential plugin, an unknown host or no
// route to the API read here as in every other view. A 400 has no kind of
// its own there: it is a server that does not know a field selector, and
// the plan falls back on it, so it is told apart first.
func classify(err error) State {
	switch {
	case err == nil:
		return StateOK
	case errors.Is(err, errNoREST):
		return StateAbsent
	case apierrors.IsBadRequest(err):
		return StateUnsupported
	}
	switch k8s.Classify(err).Kind {
	case k8s.KindForbidden:
		return StateForbidden
	case k8s.KindNotFound:
		return StateAbsent
	case k8s.KindUnavailable:
		return StateNotReady
	case k8s.KindThrottled:
		return StateThrottled
	case k8s.KindUnauthorized, k8s.KindLogin:
		return StateUnauthorized
	case k8s.KindTimeout, k8s.KindUnreachable:
		return StateStale
	default:
		return StateFailed
	}
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
	// Answered is set while the snapshot holds an answer of this endpoint:
	// from its first OK until a state that drops the data (or a new scope).
	Answered bool
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
