package async

import (
	"strconv"
	"time"
)

// MaxBackoff caps the interval a backoff can reach. A base interval above
// it, or a longer Retry-After, still wins.
const MaxBackoff = 60 * time.Second

// maxSteps bounds the step counters; the doubling stops at MaxBackoff long
// before.
const maxSteps = 16

// backoff is the poller's memory of how the API has been answering.
type backoff struct {
	slow       int // consecutive Slow outcomes
	throttle   int // doublings asked for by Throttled outcomes
	retryAfter time.Duration
}

// observe folds one outcome in. retryAfter only ever floors the next tick.
func (b backoff) observe(o Outcome, retryAfter time.Duration) backoff {
	next := backoff{slow: b.slow, throttle: b.throttle, retryAfter: retryAfter}
	switch o {
	case OK:
		next.slow = 0
		next.throttle = max(b.throttle-1, 0)
	case Slow:
		next.slow = min(b.slow+1, maxSteps)
	case Throttled:
		next.throttle = min(b.throttle+1, maxSteps)
	}
	return next
}

// delay doubles base once per step, up to MaxBackoff, then applies the
// Retry-After floor. With a 5s base the steps are 5, 10, 20, 40, 60s.
func (b backoff) delay(base time.Duration) time.Duration {
	d := base
	for range b.slow + b.throttle {
		if d >= MaxBackoff {
			break
		}
		d *= 2
	}
	if d > MaxBackoff {
		d = max(MaxBackoff, base)
	}
	return max(d, b.retryAfter)
}

func (b backoff) reason() string {
	if b.throttle > 0 {
		return "throttled"
	}
	return "slow API"
}

// seconds renders a cadence the way the footer shows it: "5s", "60s".
func seconds(d time.Duration) string {
	if d < time.Second {
		return d.String()
	}
	return strconv.FormatInt(int64(d.Round(time.Second)/time.Second), 10) + "s"
}
