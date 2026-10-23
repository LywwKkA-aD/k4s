package dashboard

import (
	"slices"
	"sync"
	"time"
)

const (
	// renderSamples is how many renders the slow-render switch looks at.
	renderSamples = 16
	// renderWarmup is how many first renders of a dashboard it skips: they
	// draw everything cold (the styles, the gradients, the first
	// snapshot) and say nothing about the frames that follow.
	renderWarmup = 2
	// slowRender is the median render time past which motion is switched
	// off: frames that slow would cost more than they show.
	slowRender = 4 * time.Millisecond
)

// renderClock keeps the times of the last renderSamples renders after the
// warm-up, so the switch it drives clears again when renders get faster.
// Each dashboard has its own: a slow run is not carried to the next one.
// View is a value method and cannot change the model, so the model holds
// this one pointer and View writes to it; the lock makes that safe
// wherever View runs.
type renderClock struct {
	mu      sync.Mutex
	samples [renderSamples]time.Duration
	n       int // renders recorded in samples
	seen    int // renders drawn, the warm-up included
}

// record notes one render that took d. A nil clock records nothing.
func (c *renderClock) record(d time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seen++
	if c.seen <= renderWarmup {
		return
	}
	c.samples[c.n%renderSamples] = d
	c.n++
}

// renders is how many renders were drawn.
func (c *renderClock) renders() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seen
}

// slow reports a median above slowRender over the last renderSamples
// renders; never before that many were recorded after the warm-up.
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
