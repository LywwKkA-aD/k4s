package tuitest

import (
	"sync/atomic"

	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
)

// Outage is a switch for API failures that a test flips between steps: the
// list that worked fails from Down on and works again after Up. The zero
// value is up and safe for concurrent use.
type Outage struct {
	down atomic.Bool
}

// Down makes every call guarded by o fail.
func (o *Outage) Down() { o.down.Store(true) }

// Up lets the calls guarded by o through again.
func (o *Outage) Up() { o.down.Store(false) }

func (o *Outage) isDown() bool { return o != nil && o.down.Load() }

// Lists fails every list of resource with err while o is down.
func (o *Outage) Lists(resource string, err error) FakeOption {
	return WithReactor("list", resource, func(k8stesting.Action) (bool, runtime.Object, error) {
		if o.isDown() {
			return true, nil, err
		}
		return false, nil, nil
	})
}

// Metrics makes the metrics.k8s.io API answer 503 while o is down.
func (o *Outage) Metrics() FakeOption {
	return func(c fakeConfig) fakeConfig {
		c.metricsOutage = o
		return c
	}
}
