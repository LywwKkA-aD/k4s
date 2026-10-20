package k8stest

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ktesting "k8s.io/client-go/testing"
)

// User is the identity named in every Forbidden error built here: the
// service account of the restricted dev-cluster fixture.
const User = "system:serviceaccount:k4s-demo:k4s-viewer"

// fakeServer is the apiserver URL quoted in errors from fake reactors,
// which have no real server behind them.
const fakeServer = "https://k8stest.invalid:6443"

// request is what the apiserver's authorizer sees: enough to word a denial
// or rebuild the URL client-go would have called.
type request struct {
	verb        string
	group       string
	version     string
	resource    string
	subresource string
	namespace   string
	name        string
	path        string // set only for non-resource requests such as /version
}

func requestFromAction(a ktesting.Action) request {
	gvr := a.GetResource()
	return request{
		verb:        a.GetVerb(),
		group:       gvr.Group,
		version:     gvr.Version,
		resource:    gvr.Resource,
		subresource: a.GetSubresource(),
		namespace:   a.GetNamespace(),
		name:        nameOf(a),
	}
}

// nameOf returns the object name the request URL carries. An update names
// its object only in the body; a create targets the collection, so it has
// none even though its body does.
func nameOf(a ktesting.Action) string {
	if named, ok := a.(interface{ GetName() string }); ok {
		return named.GetName()
	}
	update, ok := a.(ktesting.UpdateAction)
	if !ok || a.GetVerb() != "update" {
		return ""
	}
	obj, err := meta.Accessor(update.GetObject())
	if err != nil {
		return ""
	}
	return obj.GetName()
}

// requestFromHTTP parses /api/{v}/... and /apis/{g}/{v}/... paths; anything
// else is a non-resource request.
func requestFromHTTP(r *http.Request) request {
	segs := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	var q request
	var rest []string
	switch {
	case len(segs) >= 3 && segs[0] == "api":
		q.version, rest = segs[1], segs[2:]
	case len(segs) >= 4 && segs[0] == "apis":
		q.group, q.version, rest = segs[1], segs[2], segs[3:]
	default:
		q.path = r.URL.Path
		q.verb = strings.ToLower(r.Method)
		return q
	}
	if len(rest) > 2 && rest[0] == "namespaces" {
		q.namespace, rest = rest[1], rest[2:]
	}
	q.resource = rest[0]
	if len(rest) > 1 {
		q.name = rest[1]
	}
	if len(rest) > 2 {
		q.subresource = rest[2]
	}
	q.verb = verbFor(r, q.name)
	return q
}

func verbFor(r *http.Request, name string) string {
	switch r.Method {
	case http.MethodPost:
		return "create"
	case http.MethodPut:
		return "update"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		if name == "" {
			return "deletecollection"
		}
		return "delete"
	}
	switch {
	case r.URL.Query().Get("watch") == "true":
		return "watch"
	case name == "":
		return "list"
	default:
		return "get"
	}
}

// forbidden words the denial of User exactly as the apiserver's authorization
// filter does, so assertions on messages match a real cluster.
func (q request) forbidden() *apierrors.StatusError {
	if q.path != "" {
		reason := fmt.Sprintf("User %q cannot %s path %q", User, q.verb, q.path)
		return apierrors.NewForbidden(schema.GroupResource{}, "", errors.New(reason))
	}
	resource := q.resource
	if q.subresource != "" {
		resource += "/" + q.subresource
	}
	scope := "at the cluster scope"
	if q.namespace != "" {
		scope = fmt.Sprintf("in the namespace %q", q.namespace)
	}
	reason := fmt.Sprintf("User %q cannot %s resource %q in API group %q %s", User, q.verb, resource, q.group, scope)
	gr := schema.GroupResource{Group: q.group, Resource: q.resource}
	return apierrors.NewForbidden(gr, q.name, errors.New(reason))
}

// url rebuilds the request URL against fakeServer.
func (q request) url() string {
	var b strings.Builder
	b.WriteString(fakeServer)
	if q.group == "" {
		b.WriteString("/api/" + q.version)
	} else {
		b.WriteString("/apis/" + q.group + "/" + q.version)
	}
	if q.namespace != "" {
		b.WriteString("/namespaces/" + q.namespace)
	}
	b.WriteString("/" + q.resource)
	for _, s := range []string{q.name, q.subresource} {
		if s != "" {
			b.WriteString("/" + s)
		}
	}
	return b.String()
}

// httpOp is the url.Error Op net/http reports for the method behind verb.
func httpOp(verb string) string {
	switch verb {
	case "create":
		return "Post"
	case "update":
		return "Put"
	case "patch":
		return "Patch"
	case "delete", "deletecollection":
		return "Delete"
	default:
		return "Get"
	}
}
