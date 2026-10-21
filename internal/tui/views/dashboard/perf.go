package dashboard

import (
	"slices"
	"sync"
	"time"
)

const (
	// renderSamples is how many renders the slow-render switch looks at.
	renderSamples = 16
	// slowRender is the median render time past which motion is switched
	// off for the session: frames that slow would cost more than they show.
	slowRender = 4 * time.Millisecond
)

// renderClock keeps the times of the last renderSamples renders. View
// is a value method and cannot change the model, so the model holds this
// one pointer and View writes to it; the lock makes that safe wherever
// View runs.
type renderClock struct {
	mu      sync.Mutex
	samples [renderSamples]time.Duration
	n       int
}

// record notes one render that took d. A nil clock records nothing.
func (c *renderClock) record(d time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.samples[c.n%renderSamples] = d
	c.n++
}

// slow reports a median above slowRender over the last renderSamples
// renders; never before that many were recorded.
func (c *renderClock) slow() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.n < renderSamples {
		return false
	}
	sorted := slices.Clone(c.samples[:])
	slices.Sort(sorted)
	return (sorted[renderSamples/2-1]+sorted[renderSamples/2])/2 > slowRender
}
