package k8s

import (
	"context"
	"errors"
	"fmt"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Stats is a coarse-grained snapshot of cluster contents, suitable for the
// landing dashboard. It is not authoritative — for the per-resource views we
// will eventually use informer-backed watchers.
type Stats struct {
	Namespaces  int
	Pods        int
	Deployments int
	Services    int
	// Failed lists the resources whose count could not be fetched (RBAC
	// denial, timeout, …). Their counters stay at zero while the rest of
	// the dashboard still renders — one missing permission should not
	// blank the whole screen. Empty means every count succeeded.
	Failed []string
}

// countListOptions asks the apiserver for a single item per resource; the
// real count is derived from ListMeta.RemainingItemCount. On a big cluster
// this turns four multi-megabyte full lists into four tiny responses.
var countListOptions = metav1.ListOptions{Limit: 1}

// Stats issues four count queries (namespaces, pods, deployments, services)
// concurrently and returns per-resource counts. A resource that errors lands
// in Stats.Failed; the overall error is non-nil only when *every* query
// failed (e.g. the apiserver is unreachable), so partial RBAC still yields
// a usable dashboard.
func (c *Client) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	var mu sync.Mutex
	var wg sync.WaitGroup
	var errs []error

	// collect runs one count query and records the outcome. dst is written
	// under mu; on error the resource name joins Stats.Failed instead.
	collect := func(name string, dst *int, list func(context.Context) (int, *int64, error)) {
		defer wg.Done()
		items, remaining, err := list(ctx)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			s.Failed = append(s.Failed, name)
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			return
		}
		n := items
		if remaining != nil {
			n += int(*remaining)
		}
		*dst = n
	}

	wg.Add(4)
	go collect("namespaces", &s.Namespaces, func(ctx context.Context) (int, *int64, error) {
		l, err := c.Clientset.CoreV1().Namespaces().List(ctx, countListOptions)
		if err != nil {
			return 0, nil, err
		}
		return len(l.Items), l.RemainingItemCount, nil
	})
	go collect("pods", &s.Pods, func(ctx context.Context) (int, *int64, error) {
		l, err := c.Clientset.CoreV1().Pods("").List(ctx, countListOptions)
		if err != nil {
			return 0, nil, err
		}
		return len(l.Items), l.RemainingItemCount, nil
	})
	go collect("deployments", &s.Deployments, func(ctx context.Context) (int, *int64, error) {
		l, err := c.Clientset.AppsV1().Deployments("").List(ctx, countListOptions)
		if err != nil {
			return 0, nil, err
		}
		return len(l.Items), l.RemainingItemCount, nil
	})
	go collect("services", &s.Services, func(ctx context.Context) (int, *int64, error) {
		l, err := c.Clientset.CoreV1().Services("").List(ctx, countListOptions)
		if err != nil {
			return 0, nil, err
		}
		return len(l.Items), l.RemainingItemCount, nil
	})
	wg.Wait()

	if len(errs) == 4 {
		return s, errors.Join(errs...)
	}
	return s, nil
}
