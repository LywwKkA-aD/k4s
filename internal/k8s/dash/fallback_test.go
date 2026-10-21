package dash

import (
	"net/http"
	"testing"
)

// podsDeniedClusterWide denies only the all-namespaces pods list, so that
// one refresh mixes -A lists with a pods list that fell back to a namespace.
func podsDeniedClusterWide() option {
	return func(a *apiserver) {
		a.gate = func(r *http.Request) bool { return r.URL.Path == "/api/v1/pods" }
	}
}

func TestFallbackRefreshResolvesEachListInItsOwnNamespace(t *testing.T) {
	t.Parallel()

	a := newAPIServer(t, busyK3s(fixedNow), podsDeniedClusterWide())
	s := tickAt(t, a, NewSnapshot(Scope{Context: "shop"}), 0)
	if !s.Scope.Fallback || s.Scope.Namespace != "shop" {
		t.Fatalf("scope %+v, want the pods list to fall back to shop", s.Scope)
	}
	var shopCrash, worker, stagingAPI bool
	for _, r := range s.Pods.Problems {
		if r.Key.Name == "payments-worker-6c9f7d8b5-xk2lp" {
			shopCrash = r.Resolved && r.Key.Namespace == "shop" && r.Container == "worker"
		}
	}
	for _, r := range s.Workloads.Rows {
		switch {
		case r.Key.Kind == "deployment" && r.Key.Name == "payments-worker":
			worker = r.Resolved && r.Key.Namespace == "shop"
		case r.Key.Kind == "deployment" && r.Key.Name == "api":
			stagingAPI = r.Resolved && r.Key.Namespace == "staging"
		}
	}
	if !shopCrash || !worker || !stagingAPI {
		t.Fatalf("crash pod placed %v, payments-worker %v, staging api %v", shopCrash, worker, stagingAPI)
	}
}
