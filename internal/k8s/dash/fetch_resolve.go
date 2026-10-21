package dash

import (
	"context"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// lookup is one finished typed lookup.
type lookup struct {
	key   resolveKey
	entry resolveEntry
	bytes int
	keep  bool // cache the entry; transient failures are retried next refresh
}

func (f Fetcher) lookupAll(ctx context.Context, todo []want, scope Scope, consistent bool) []lookup {
	out := make([]lookup, len(todo))
	sem := make(chan struct{}, maxInFlight)
	var wg sync.WaitGroup
	for i, w := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			cctx, cancel := context.WithTimeout(ctx, clusterTimeout)
			defer cancel()
			objs, n, err := f.lookupOne(cctx, w.key, scope.Namespace, consistent)
			st := classify(err)
			out[i] = lookup{
				key: w.key, entry: resolveEntry{sig: w.sig, objs: objs}, bytes: n,
				keep: st == StateOK || st == StateForbidden,
			}
		}()
	}
	wg.Wait()
	return out
}

// lister lists one kind by name and keeps what a row needs of each match.
// The name is re-checked: a fake clientset ignores field selectors.
type lister func(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error)

var listers = map[string]lister{
	"pod":         lookupPods,
	"deployment":  lookupDeployments,
	"statefulset": lookupStatefulSets,
	"daemonset":   lookupDaemonSets,
	"cronjob":     lookupCronJobs,
	"service":     lookupServices,
}

// lookupOne lists objects of key.kind named key.name, in ns or across all
// namespaces.
func (f Fetcher) lookupOne(ctx context.Context, key resolveKey, ns string, consistent bool) ([]resolvedObj, int, error) {
	opts := metav1.ListOptions{FieldSelector: "metadata.name=" + key.name, ResourceVersion: "0"}
	if consistent {
		opts.ResourceVersion = ""
	}
	return listers[key.kind](ctx, f.src.Typed, ns, key.name, opts)
}

func lookupPods(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.CoreV1().Pods(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, p := range l.Items {
		if p.Name == name {
			out = append(out, resolvedPod(p))
		}
	}
	return out, l.Size(), nil
}

func lookupDeployments(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.AppsV1().Deployments(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, d := range l.Items {
		if d.Name == name {
			out = append(out, resolvedObj{namespace: d.Namespace, ready: int(d.Status.ReadyReplicas), desired: replicas(d.Spec.Replicas)})
		}
	}
	return out, l.Size(), nil
}

func lookupStatefulSets(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.AppsV1().StatefulSets(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, s := range l.Items {
		if s.Name == name {
			out = append(out, resolvedObj{namespace: s.Namespace, ready: int(s.Status.ReadyReplicas), desired: replicas(s.Spec.Replicas)})
		}
	}
	return out, l.Size(), nil
}

func lookupDaemonSets(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.AppsV1().DaemonSets(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, d := range l.Items {
		if d.Name == name {
			out = append(out, resolvedObj{namespace: d.Namespace, ready: int(d.Status.NumberReady), desired: int(d.Status.DesiredNumberScheduled)})
		}
	}
	return out, l.Size(), nil
}

func lookupCronJobs(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.BatchV1().CronJobs(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, c := range l.Items {
		if c.Name == name {
			out = append(out, resolvedObj{namespace: c.Namespace})
		}
	}
	return out, l.Size(), nil
}

func lookupServices(ctx context.Context, cs kubernetes.Interface, ns, name string, o metav1.ListOptions) ([]resolvedObj, int, error) {
	l, err := cs.CoreV1().Services(ns).List(ctx, o)
	if err != nil {
		return nil, 0, err
	}
	var out []resolvedObj
	for _, s := range l.Items {
		if s.Name == name {
			out = append(out, resolvedObj{namespace: s.Namespace})
		}
	}
	return out, l.Size(), nil
}

// replicas is spec.replicas, which defaults to 1.
func replicas(n *int32) int {
	if n == nil {
		return 1
	}
	return int(*n)
}
