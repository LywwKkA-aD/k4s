package k8stest

import (
	"context"
	"net/url"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	ktesting "k8s.io/client-go/testing"
)

// Reactor is the part of a client-go fake the helpers install into.
// *fake.Clientset satisfies it, as does every fake built on testing.Fake.
type Reactor interface {
	PrependReactor(verb, resource string, reaction ktesting.ReactionFunc)
	PrependWatchReactor(resource string, reaction ktesting.WatchReactionFunc)
}

// RestrictToNamespace makes r answer like a client whose only grant is a
// Role in namespace. Requests inside namespace fall through to the reactors
// and object tracker already installed; everything else is Forbidden with
// the apiserver's wording: lists across all namespaces (-A), requests in
// other namespaces, and cluster-scoped resources such as namespaces and
// nodes, which a Role can never grant.
func RestrictToNamespace(r Reactor, namespace string) {
	r.PrependReactor("*", "*", func(a ktesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() == namespace {
			return false, nil, nil
		}
		return true, nil, requestFromAction(a).forbidden()
	})
	r.PrependWatchReactor("*", func(a ktesting.Action) (bool, watch.Interface, error) {
		if a.GetNamespace() == namespace {
			return false, nil, nil
		}
		return true, nil, requestFromAction(a).forbidden()
	})
}

// Forbid denies verb on resource in every namespace and at cluster scope,
// as when RBAC grants nothing for it; "*" matches any verb or resource.
// Other requests fall through.
func Forbid(r Reactor, verb, resource string) {
	r.PrependReactor(verb, resource, func(a ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, requestFromAction(a).forbidden()
	})
	if verb != "*" && verb != "watch" {
		return
	}
	r.PrependWatchReactor(resource, func(a ktesting.Action) (bool, watch.Interface, error) {
		return true, nil, requestFromAction(a).forbidden()
	})
}

// Unauthorized fails every request on r with 401 Unauthorized, which is how
// the apiserver answers an expired or revoked bearer token.
func Unauthorized(r Reactor) {
	r.PrependReactor("*", "*", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewUnauthorized("Unauthorized")
	})
	r.PrependWatchReactor("*", func(ktesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewUnauthorized("Unauthorized")
	})
}

// SlowerThanDeadline makes every request on r except watches stall for
// after and then fail the way client-go reports a context deadline that
// passed before the apiserver answered: a *url.Error wrapping
// context.DeadlineExceeded. A fake clientset never sees the caller's
// context, so the stall happens whatever deadline the caller set; pick
// after just past the deadline under test. For a server that really holds
// the connection until the caller's context ends, use Hang with
// NewServerClient.
func SlowerThanDeadline(r Reactor, after time.Duration) {
	r.PrependReactor("*", "*", func(a ktesting.Action) (bool, runtime.Object, error) {
		time.Sleep(after)
		q := requestFromAction(a)
		return true, nil, &url.Error{Op: httpOp(q.verb), URL: q.url(), Err: context.DeadlineExceeded}
	})
}
