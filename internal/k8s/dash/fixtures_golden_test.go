package dash

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"
)

var watching = Focus{Active: true, TopPods: true}

// refresh runs the first load and two ticks after it, by when the k3s
// endpoints and the capped typed lookups have caught up.
func refresh(t *testing.T, a *apiserver, scope Scope) Snapshot {
	t.Helper()
	f := NewFetcher(a.src).WithClock(func() time.Time { return fixedNow })
	s := NewSnapshot(scope)
	for _, now := range []time.Duration{0, 5 * sec, 10 * sec} {
		s = f.Fetch(context.Background(), MakePlan(s.PlanInput(now, true, false, watching)), s)
	}
	return s
}

// scopedTo grants a namespace-scoped user only ns, plus what every
// authenticated user may do: /version, /readyz, discovery and whoami.
func scopedTo(ns string) option {
	return func(a *apiserver) {
		a.cs.PrependReactor("*", "*", func(act ktesting.Action) (bool, runtime.Object, error) {
			res := act.GetResource().Resource
			if res == "version" || res == "resource" || res == "selfsubjectreviews" || act.GetNamespace() == ns {
				return false, nil, nil
			}
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: res}, "", errors.New("scoped user"))
		})
		a.gate = func(r *http.Request) bool {
			return r.URL.Path != "/readyz" && !strings.Contains(r.URL.Path, "/namespaces/"+ns+"/")
		}
	}
}

func TestFixtureGoldens(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		c     cluster
		scope Scope
		opts  []option
	}{
		{"busy-k3s", busyK3s(fixedNow), Scope{}, nil},
		{"all-clear", allClearK3s(fixedNow), Scope{}, nil},
		{"no-metrics-k3s", noMetricsK3s(fixedNow), Scope{}, nil},
		{"vanilla-kind", vanillaKind(fixedNow), Scope{}, nil},
		{"ns-scoped-eks", nsScopedEKS(fixedNow), Scope{Context: "shop"}, []option{scopedTo("shop")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := refresh(t, newAPIServer(t, tc.c, tc.opts...), tc.scope)
			golden(t, tc.name, render(s))
		})
	}
}
