package async_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"syscall"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

type netTimeout struct{}

func (netTimeout) Error() string   { return "i/o timeout" }
func (netTimeout) Timeout() bool   { return true }
func (netTimeout) Temporary() bool { return true }

func TestClassify(t *testing.T) {
	t.Parallel()
	pods := schema.GroupResource{Resource: "pods"}
	refused := &url.Error{Op: "Get", URL: "https://127.0.0.1:6443/api/v1/pods", Err: &net.OpError{
		Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED,
	}}

	tests := []struct {
		name       string
		err        error
		outcome    async.Outcome
		retryAfter time.Duration
	}{
		{name: "nil", err: nil, outcome: async.OK},
		{name: "context deadline", err: context.DeadlineExceeded, outcome: async.Slow},
		{name: "wrapped deadline", err: fmt.Errorf("list pods: %w", &url.Error{Op: "Get", Err: context.DeadlineExceeded}), outcome: async.Slow},
		{name: "network timeout", err: &url.Error{Op: "Get", Err: netTimeout{}}, outcome: async.Slow},
		{name: "504 gateway timeout", err: apierrors.NewTimeoutError("slow", 0), outcome: async.Slow},
		{name: "server timeout with retry-after", err: apierrors.NewServerTimeout(pods, "list", 3), outcome: async.Slow, retryAfter: 3 * time.Second},
		{name: "connection refused", err: fmt.Errorf("list pods: %w", refused), outcome: async.Slow},
		{name: "429", err: apierrors.NewTooManyRequestsError("slow down"), outcome: async.Throttled},
		{name: "429 with retry-after", err: apierrors.NewTooManyRequests("slow down", 7), outcome: async.Throttled, retryAfter: 7 * time.Second},
		{name: "wrapped 429", err: fmt.Errorf("list pods: %w", apierrors.NewTooManyRequests("slow down", 2)), outcome: async.Throttled, retryAfter: 2 * time.Second},
		{name: "503 with retry-after", err: apierrors.NewGenericServerResponse(503, "list", pods, "", "", 9, false), outcome: async.Throttled, retryAfter: 9 * time.Second},
		{name: "503 without retry-after", err: apierrors.NewServiceUnavailable("not ready"), outcome: async.Failed},
		{name: "forbidden", err: apierrors.NewForbidden(pods, "", errors.New("rbac")), outcome: async.Failed},
		{name: "not found", err: apierrors.NewNotFound(pods, "web"), outcome: async.Failed},
		{name: "plain error", err: errors.New("boom"), outcome: async.Failed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outcome, retryAfter := async.Classify(tc.err)
			if outcome != tc.outcome || retryAfter != tc.retryAfter {
				t.Fatalf("Classify = (%d, %v), want (%d, %v)", outcome, retryAfter, tc.outcome, tc.retryAfter)
			}
		})
	}
}
