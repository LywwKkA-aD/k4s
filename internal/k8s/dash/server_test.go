package dash

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	authv1 "k8s.io/api/authentication/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/LywwKkA-aD/k4s/internal/k8s/k8stest"
)

// seen is one request the fake apiserver answered, typed or over HTTP.
type seen struct {
	typed         bool
	verb          string
	resource      string // resource for typed calls, URL path for HTTP
	ns            string
	rv            string
	hasRV         bool
	limit         int64
	fieldSelector string
}

// apiserver serves a cluster: typed calls from a fake clientset with
// reactors, Tables, raw paths and metrics from httptest. It logs every
// request with its list options.
type apiserver struct {
	c   cluster
	cs  *fake.Clientset
	src Source

	mu        sync.Mutex
	log       []seen
	overrides map[string]http.Handler  // URL path prefix
	typedErrs map[string]error         // "verb resource" or "* resource"
	gate      func(*http.Request) bool // true denies the request with 403
}

// option adjusts the fake before the counting reactor goes in front, so
// requests that RBAC reactors deny are still counted.
type option func(*apiserver)

func newAPIServer(t testing.TB, c cluster, opts ...option) *apiserver {
	t.Helper()
	a := &apiserver{c: c, overrides: map[string]http.Handler{}, typedErrs: map[string]error{}}
	a.cs = fake.NewClientset(c.objects()...)
	a.cs.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: c.git, Major: "1", Minor: minorString(c.git)}
	if c.metrics {
		a.cs.Resources = []*metav1.APIResourceList{{
			GroupVersion: metricsGroupVersion,
			APIResources: []metav1.APIResource{{Name: "nodes", Kind: "NodeMetrics"}, {Name: "pods", Kind: "PodMetrics", Namespaced: true}},
		}}
	}
	a.cs.PrependReactor("create", "selfsubjectreviews", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, &authv1.SelfSubjectReview{Status: authv1.SelfSubjectReviewStatus{UserInfo: authv1.UserInfo{
			Username: c.user, Extra: map[string]authv1.ExtraValue{"token-secret": {"extra-must-not-show"}},
		}}}, nil
	})
	a.cs.PrependReactor("list", "*", a.listReactor)
	a.cs.PrependReactor("*", "*", a.errReactor)
	for _, o := range opts {
		o(a)
	}
	a.cs.PrependReactor("*", "*", a.countReactor)

	srv := httptest.NewServer(http.HandlerFunc(a.serveHTTP))
	t.Cleanup(func() {
		srv.CloseClientConnections()
		srv.Close()
	})
	a.src = Source{Typed: a.cs, REST: restFor(t, srv.URL), Host: srv.URL}
	return a
}

func minorString(git string) string {
	parts := strings.SplitN(strings.TrimPrefix(git, "v"), ".", 3)
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

func (a *apiserver) countReactor(act ktesting.Action) (bool, runtime.Object, error) {
	s := seen{typed: true, verb: act.GetVerb(), resource: act.GetResource().Resource, ns: act.GetNamespace()}
	if l, ok := act.(ktesting.ListActionImpl); ok {
		s.rv, s.hasRV, s.limit = l.ListOptions.ResourceVersion, true, l.ListOptions.Limit
		s.fieldSelector = l.ListOptions.FieldSelector
	}
	a.mu.Lock()
	a.log = append(a.log, s)
	a.mu.Unlock()
	return false, nil, nil
}

func (a *apiserver) errReactor(act ktesting.Action) (bool, runtime.Object, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, k := range []string{act.GetVerb() + " " + act.GetResource().Resource, "* " + act.GetResource().Resource} {
		if err, ok := a.typedErrs[k]; ok {
			return true, nil, err
		}
	}
	return false, nil, nil
}

// listReactor applies field selectors and limits, which the fake
// clientset's tracker ignores, so lookups and capped lists cost what they
// would on a real server.
func (a *apiserver) listReactor(act ktesting.Action) (bool, runtime.Object, error) {
	l, ok := act.(ktesting.ListActionImpl)
	if !ok || (l.ListOptions.FieldSelector == "" && l.ListOptions.Limit == 0) {
		return false, nil, nil
	}
	obj, err := a.cs.Tracker().List(l.GetResource(), l.GetKind(), l.GetNamespace())
	if err != nil {
		return true, nil, err
	}
	items, err := meta.ExtractList(obj)
	if err != nil {
		return true, nil, err
	}
	sel, err := fields.ParseSelector(l.ListOptions.FieldSelector)
	if err != nil {
		return true, nil, err
	}
	kept := make([]runtime.Object, 0, len(items))
	for _, it := range items {
		if sel.Matches(fieldsOf(it)) {
			kept = append(kept, it)
		}
	}
	if lim := int(l.ListOptions.Limit); lim > 0 && len(kept) > lim {
		rest := int64(len(kept) - lim)
		kept = kept[:lim]
		if lm, err := meta.ListAccessor(obj); err == nil {
			lm.SetContinue("next")
			lm.SetRemainingItemCount(&rest)
		}
	}
	if err := meta.SetList(obj, kept); err != nil {
		return true, nil, err
	}
	return true, obj, nil
}

func fieldsOf(obj runtime.Object) fields.Set {
	m, _ := meta.Accessor(obj)
	set := fields.Set{"metadata.name": m.GetName(), "metadata.namespace": m.GetNamespace()}
	switch o := obj.(type) {
	case *corev1.Event:
		set["type"] = o.Type
	case *batchv1.Job:
		set["status.successful"] = strconv.Itoa(int(o.Status.Succeeded))
	case *corev1.Service:
		set["spec.type"] = string(o.Spec.Type)
	}
	return set
}

func (a *apiserver) serveHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	s := seen{verb: "get", resource: r.URL.Path, fieldSelector: q.Get("fieldSelector")}
	s.rv, s.hasRV = q.Get("resourceVersion"), q.Has("resourceVersion")
	s.limit, _ = strconv.ParseInt(q.Get("limit"), 10, 64)
	a.mu.Lock()
	a.log = append(a.log, s)
	var override http.Handler
	for prefix, h := range a.overrides {
		if strings.HasPrefix(r.URL.Path, prefix) {
			override = h
		}
	}
	if a.gate != nil && a.gate(r) {
		override = k8stest.Forbidden()
	}
	a.mu.Unlock()
	if override != nil {
		override.ServeHTTP(w, r)
		return
	}
	a.answer(w, r)
}

func (a *apiserver) answer(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case path == "/readyz":
		body := a.c.readyz
		if strings.Contains(body, "check failed") {
			w.WriteHeader(http.StatusInternalServerError)
		}
		_, _ = fmt.Fprint(w, body)
		return
	case strings.HasPrefix(path, "/apis/"+metricsGroupVersion+"/"):
		if !a.c.metrics {
			http.NotFound(w, r)
			return
		}
		res, ns := splitPath(strings.TrimPrefix(path, "/apis/"+metricsGroupVersion+"/"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(a.c.metricsJSON(res, ns))
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "as=Table") {
		http.Error(w, "this fake serves Tables only", http.StatusNotAcceptable)
		return
	}
	trimmed := strings.TrimPrefix(strings.TrimPrefix(path, "/api/v1/"), "/apis/")
	for _, gv := range []string{"apps/v1/", "batch/v1/", "helm.cattle.io/v1/", "k3s.cattle.io/v1/"} {
		trimmed = strings.TrimPrefix(trimmed, gv)
	}
	res, ns := splitPath(trimmed)
	t, ok := a.c.table(res, ns)
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSONBody(w, t)
}

// splitPath reads "namespaces/<ns>/<res>" or "<res>".
func splitPath(p string) (resource, ns string) {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 3 && parts[0] == "namespaces" {
		return parts[2], parts[1]
	}
	return parts[len(parts)-1], ""
}

// take returns and clears the requests seen so far.
func (a *apiserver) take() []seen {
	a.mu.Lock()
	defer a.mu.Unlock()
	log := a.log
	a.log = nil
	return log
}

func (a *apiserver) override(prefix string, h http.Handler) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.overrides[prefix] = h
}

func (a *apiserver) failTyped(key string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.typedErrs[key] = err
}
