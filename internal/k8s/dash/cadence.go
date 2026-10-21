package dash

import "time"

// Cadence constants from the spec (§8 polling rules) and the critique's
// per-endpoint size rules.
const (
	base           = 5 * time.Second  // fast tier and tick granularity
	mediumEvery    = 15 * time.Second // medium tier
	slowEvery      = 30 * time.Second // slow tier
	minuteEvery    = 60 * time.Second
	parkFor        = 5 * time.Minute // 403 and 404 re-probe
	etcdEvery      = 5 * time.Minute // k3s snapshots run every 12 h by default
	idleAfter      = 10 * time.Minute
	slowWall       = time.Second
	manyRows       = 1000
	hugeRows       = 5000
	bigBody        = 1 << 20
	hugeBody       = 4 << 20
	bigList        = 2000
	bigListBody    = 2 << 20
	manyNodes      = 20
	eventCap       = 2000 // above this many warnings, list a capped page every minute
	maxBackoffWait = 60 * time.Second
)

// backoffSteps is the M3.3 sequence for consecutive failed refreshes.
var backoffSteps = []time.Duration{base, 10 * time.Second, 20 * time.Second, 40 * time.Second, maxBackoffWait}

// intervalOf is how often e is due; -1 means once per context.
func intervalOf(e Endpoint, in PlanInput) time.Duration {
	switch e.Tier() {
	case Fast:
		return fastInterval(in)
	case Medium:
		return mediumInterval(e, in)
	case Slow:
		return slowInterval(e, in)
	default:
		if e == MetricsDiscovery && !in.Caps.Metrics {
			return minuteEvery
		}
		return -1
	}
}

// fastStretch is the fast-tier interval the focus alone asks for.
func fastStretch(f Focus) time.Duration {
	if f.Blurred || f.Idle >= idleAfter {
		return mediumEvery
	}
	return base
}

// fastInterval stretches the fast tier for blur, idle time and the size or
// wall time of the fast lists.
func fastInterval(in PlanInput) time.Duration {
	iv := fastStretch(in.Focus)
	rows, bytes, wall := 0, 0, time.Duration(0)
	for _, e := range []Endpoint{Pods, Deployments} {
		u := in.Ledger[e]
		rows, bytes, wall = max(rows, u.Rows), max(bytes, u.Bytes), max(wall, u.Wall)
	}
	switch {
	case rows > hugeRows || bytes > hugeBody:
		return max(iv, slowEvery)
	case rows > manyRows || bytes > bigBody || wall > slowWall:
		return max(iv, mediumEvery)
	default:
		return iv
	}
}

func mediumInterval(e Endpoint, in PlanInput) time.Duration {
	iv := mediumEvery
	if in.Focus.Blurred {
		iv = slowEvery
	}
	if e == PodMetrics && in.Ledger[Pods].Rows > manyRows {
		iv = slowEvery
	}
	// A tier never runs faster than the one below it.
	return max(iv, sizeStretch(in.Ledger[e], slowEvery), fastInterval(in))
}

func slowInterval(e Endpoint, in PlanInput) time.Duration {
	u := in.Ledger[e]
	switch e {
	case ServiceCount:
		return minuteEvery // a count changes rarely and must read etcd
	case EtcdSnapshots:
		return etcdEvery
	case Nodes:
		if u.Rows > manyNodes {
			return minuteEvery
		}
	}
	return max(slowEvery, sizeStretch(u, minuteEvery), fastInterval(in))
}

// sizeStretch returns stretched when the endpoint's last list was big.
func sizeStretch(u Usage, stretched time.Duration) time.Duration {
	if u.Rows > bigList || u.Bytes > bigListBody {
		return stretched
	}
	return 0
}

// backoffFor is the extra wait after 429s (doubling) or timeouts (the M3.3
// steps); zero when neither happened.
func backoffFor(throttle, failures int) time.Duration {
	var wait time.Duration
	if throttle > 0 {
		wait = base << min(throttle, 4)
	}
	if failures > 0 {
		wait = max(wait, backoffSteps[min(failures, len(backoffSteps)-1)])
	}
	return min(wait, maxBackoffWait)
}
