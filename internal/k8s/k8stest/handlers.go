package k8stest

import (
	"encoding/json"
	"net/http"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NotFound answers every request with 404 and a NotFound Status. That is
// the apiserver's answer for an API group nobody serves, such as
// metrics.k8s.io on a cluster without metrics-server.
func NotFound() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeStatus(w, metav1.Status{
			Status:  metav1.StatusFailure,
			Code:    http.StatusNotFound,
			Reason:  metav1.StatusReasonNotFound,
			Message: "the server could not find the requested resource",
			Details: &metav1.StatusDetails{},
		})
	})
}

// ServiceUnavailable answers every request with 503 and the plain-text body
// the aggregator sends when an APIService is registered but its backend is
// not ready, such as metrics.k8s.io while metrics-server is starting or down.
func ServiceUnavailable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	})
}

// Forbidden denies every request for User with 403 and a Forbidden Status
// worded for the requested path, e.g. `pods.metrics.k8s.io is forbidden:
// User "…" cannot list resource "pods" in API group "metrics.k8s.io" at
// the cluster scope`.
func Forbidden() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeStatus(w, requestFromHTTP(r).forbidden().ErrStatus)
	})
}

// Hang holds every request open until the client gives up, like an
// apiserver behind a dead route that accepted the connection and then went
// silent. Only the caller's context ends the call.
func Hang() http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})
}

func writeStatus(w http.ResponseWriter, s metav1.Status) {
	s.TypeMeta = metav1.TypeMeta{Kind: "Status", APIVersion: "v1"}
	writeJSON(w, int(s.Code), s)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
}
