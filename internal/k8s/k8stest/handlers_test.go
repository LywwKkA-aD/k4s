package k8stest_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

func serve(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), method, target, nil))
	return rec
}

func decodeStatus(t *testing.T, rec *httptest.ResponseRecorder) metav1.Status {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var s metav1.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &s); err != nil {
		t.Fatalf("decode Status %q: %v", rec.Body.String(), err)
	}
	if s.Kind != "Status" || s.APIVersion != "v1" || s.Status != metav1.StatusFailure {
		t.Fatalf("Status envelope = %+v, want kind Status, apiVersion v1, status Failure", s)
	}
	if int(s.Code) != rec.Code {
		t.Fatalf("Status.Code = %d, HTTP code = %d, want equal", s.Code, rec.Code)
	}
	return s
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	rec := serve(t, k8stest.NotFound(), http.MethodGet, "/apis/metrics.k8s.io/v1beta1/pods")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rec.Code)
	}
	s := decodeStatus(t, rec)
	if s.Reason != metav1.StatusReasonNotFound || s.Message != "the server could not find the requested resource" {
		t.Fatalf("Status = %+v, want NotFound with the apiserver's message", s)
	}
}

func TestServiceUnavailable(t *testing.T) {
	t.Parallel()

	rec := serve(t, k8stest.ServiceUnavailable(), http.MethodGet, "/apis/metrics.k8s.io/v1beta1/nodes")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain like the aggregator", ct)
	}
	if body := strings.TrimSpace(rec.Body.String()); body != "service unavailable" {
		t.Fatalf("body = %q, want %q", body, "service unavailable")
	}
}

func TestForbidden(t *testing.T) {
	t.Parallel()

	user := k8stest.User
	cases := []struct {
		name   string
		method string
		target string
		want   string
	}{
		{
			name: "cluster-wide metrics list", method: http.MethodGet, target: "/apis/metrics.k8s.io/v1beta1/pods",
			want: `pods.metrics.k8s.io is forbidden: User "` + user + `" cannot list resource "pods" in API group "metrics.k8s.io" at the cluster scope`,
		},
		{
			name: "namespaced metrics list", method: http.MethodGet, target: "/apis/metrics.k8s.io/v1beta1/namespaces/demo/pods",
			want: `pods.metrics.k8s.io is forbidden: User "` + user + `" cannot list resource "pods" in API group "metrics.k8s.io" in the namespace "demo"`,
		},
		{
			name: "core watch", method: http.MethodGet, target: "/api/v1/pods?watch=true",
			want: `pods is forbidden: User "` + user + `" cannot watch resource "pods" in API group "" at the cluster scope`,
		},
		{
			name: "named subresource", method: http.MethodGet, target: "/api/v1/namespaces/demo/pods/web/log",
			want: `pods "web" is forbidden: User "` + user + `" cannot get resource "pods/log" in API group "" in the namespace "demo"`,
		},
		{
			name: "cluster-scoped get", method: http.MethodGet, target: "/api/v1/namespaces/demo",
			want: `namespaces "demo" is forbidden: User "` + user + `" cannot get resource "namespaces" in API group "" at the cluster scope`,
		},
		{
			name: "create", method: http.MethodPost, target: "/api/v1/namespaces/demo/pods",
			want: `cannot create resource "pods" in API group "" in the namespace "demo"`,
		},
		{
			name: "update", method: http.MethodPut, target: "/apis/apps/v1/namespaces/demo/deployments/web",
			want: `deployments.apps "web" is forbidden: User "` + user + `" cannot update resource "deployments" in API group "apps" in the namespace "demo"`,
		},
		{
			name: "patch", method: http.MethodPatch, target: "/apis/apps/v1/namespaces/demo/deployments/web",
			want: `cannot patch resource "deployments"`,
		},
		{
			name: "delete", method: http.MethodDelete, target: "/api/v1/namespaces/demo/pods/web",
			want: `cannot delete resource "pods"`,
		},
		{
			name: "delete collection", method: http.MethodDelete, target: "/api/v1/namespaces/demo/pods",
			want: `cannot deletecollection resource "pods"`,
		},
		{
			name: "non-resource path", method: http.MethodGet, target: "/version",
			want: `forbidden: User "` + user + `" cannot get path "/version"`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := serve(t, k8stest.Forbidden(), tc.method, tc.target)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("code = %d, want 403", rec.Code)
			}
			s := decodeStatus(t, rec)
			if s.Reason != metav1.StatusReasonForbidden {
				t.Fatalf("Reason = %q, want Forbidden", s.Reason)
			}
			if !strings.Contains(s.Message, tc.want) {
				t.Fatalf("Message = %q, want it to contain %q", s.Message, tc.want)
			}
		})
	}
}

func TestHangReturnsOnceTheClientGivesUp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/pods", nil)

	done := make(chan struct{})
	go func() {
		k8stest.Hang().ServeHTTP(httptest.NewRecorder(), req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Hang kept the request after its context ended")
	}
}
