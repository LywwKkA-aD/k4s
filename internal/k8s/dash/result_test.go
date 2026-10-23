package dash

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestClassify: states follow the shared classifier (k8s.Classify), so a
// credential plugin that fails, an unknown host or no route to the API
// read as the other views read them, not as a bare failure.
func TestClassify(t *testing.T) {
	t.Parallel()

	gr := schema.GroupResource{Resource: "pods"}
	cases := []struct {
		err  error
		want State
	}{
		{errors.New("getting credentials: exec: executable aws failed with exit code 255"), StateUnauthorized},
		{errors.New("getting credentials: exec: standard input is unavailable"), StateUnauthorized},
		{&url.Error{Op: "Get", URL: "https://x", Err: syscall.EHOSTUNREACH}, StateStale},
		{&url.Error{Op: "Get", URL: "https://k3s.lan:6443", Err: &net.DNSError{Err: "no such host", Name: "k3s.lan", IsNotFound: true}}, StateStale},
		{nil, StateOK},
		{errNoREST, StateAbsent},
		{apierrors.NewForbidden(gr, "", errors.New("no")), StateForbidden},
		{apierrors.NewNotFound(gr, "x"), StateAbsent},
		{apierrors.NewServiceUnavailable("starting"), StateNotReady},
		{apierrors.NewTooManyRequests("slow", 1), StateThrottled},
		{apierrors.NewBadRequest("field label not supported"), StateUnsupported},
		{apierrors.NewUnauthorized("expired"), StateUnauthorized},
		{apierrors.NewTimeoutError("slow", 1), StateStale},
		{fmt.Errorf("list: %w", context.DeadlineExceeded), StateStale},
		{&url.Error{Op: "Get", URL: "https://x", Err: syscall.ECONNREFUSED}, StateStale},
		{&url.Error{Op: "Get", URL: "https://x", Err: timeoutErr{}}, StateStale},
		{errors.New("boom"), StateFailed},
	}
	for _, tc := range cases {
		if got := classify(tc.err); got != tc.want {
			t.Errorf("classify(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
	if State(99).String() != "unknown" || StateNotReady.String() != "not ready" {
		t.Error("State.String is wrong")
	}
	if Endpoint(-1).String() != "unknown" || Pods.String() != "pods" || len(Endpoints()) != int(numEndpoints) {
		t.Error("Endpoint helpers are wrong")
	}
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "i/o timeout" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }
