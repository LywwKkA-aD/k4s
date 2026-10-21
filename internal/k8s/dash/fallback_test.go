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
	crash := podRowNamed(s, "payments-worker-6c9f7d8b5-xk2lp")
	if !crash.Resolved || crash.Key.Namespace != "shop" || crash.Container != "worker" {
		t.Fatalf("crash pod = %+v", crash)
	}
	for name, ns := range map[string]string{"payments-worker": "shop", "api": "staging"} {
		r := deploymentRowNamed(s, name)
		if !r.Resolved || r.Key.Namespace != ns {
			t.Errorf("deployment %s = %+v, want it placed in %s", name, r.Key, ns)
		}
	}
}

func podRowNamed(s Snapshot, name string) PodRow {
	for _, r := range s.Pods.Problems {
		if r.Key.Name == name {
			return r
		}
	}
	return PodRow{}
}

func deploymentRowNamed(s Snapshot, name string) WorkloadRow {
	for _, r := range s.Workloads.Rows {
		if r.Key.Kind == "deployment" && r.Key.Name == name {
			return r
		}
	}
	return WorkloadRow{}
}
