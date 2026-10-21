package dash

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"syscall"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestClassify(t *testing.T) {
	t.Parallel()

	gr := schema.GroupResource{Resource: "pods"}
	cases := []struct {
		err  error
		want State
	}{
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
