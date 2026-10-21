package async_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/async/asynctest"
)

var (
	errTimeout   = context.DeadlineExceeded
	errThrottled = apierrors.NewTooManyRequestsError("slow down")
	errForbidden = apierrors.NewForbidden(schema.GroupResource{Resource: "pods"}, "", errors.New("rbac"))
)

// delaysAfter answers one poll per outcome and returns the delay each
// reply armed.
func delaysAfter(t *testing.T, cfg async.Config, outcomes ...error) []time.Duration {
	t.Helper()
	p, clk := newPoller(cfg)
	p, _ = p.Begin()
	for _, err := range outcomes {
		p = cycle(t, p, clk, err)
	}
	return clk.Delays()
}

func secs(ns ...int) []time.Duration {
	out := make([]time.Duration, 0, len(ns))
	for _, n := range ns {
		out = append(out, time.Duration(n)*time.Second)
	}
	return out
}

func TestBackoffSequence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      async.Config
		outcomes []error
		want     []time.Duration
	}{
		{
			name:     "timeouts walk 5 10 20 40 60 and success snaps back",
			outcomes: []error{nil, errTimeout, errTimeout, errTimeout, errTimeout, errTimeout, nil},
			want:     secs(5, 10, 20, 40, 60, 60, 5),
		},
		{
			name:     "a non-timeout error keeps the cadence",
			outcomes: []error{errTimeout, errForbidden, nil},
			want:     secs(10, 10, 5),
		},
		{
			name:     "a 429 doubles the whole interval and success eases it a step at a time",
			outcomes: []error{errThrottled, errThrottled, nil, nil, nil},
			want:     secs(10, 20, 10, 5, 5),
		},
		{
			name:     "retry-after is a floor for the next tick only",
			outcomes: []error{apierrors.NewTooManyRequests("slow down", 45), nil},
			want:     secs(45, 5),
		},
		{
			name:     "a short retry-after does not undercut the doubling",
			outcomes: []error{apierrors.NewTooManyRequests("slow down", 1)},
			want:     secs(10),
		},
		{
			name:     "timeouts and a 429 stack",
			outcomes: []error{errTimeout, errThrottled, nil},
			want:     secs(10, 20, 5),
		},
		{
			name:     "the steps scale with the base interval",
			cfg:      async.Config{Interval: 15 * time.Second},
			outcomes: []error{errThrottled, errTimeout, errTimeout, nil},
			want:     secs(30, 60, 60, 15),
		},
		{
			name:     "a base above the cap is kept",
			cfg:      async.Config{Interval: 90 * time.Second},
			outcomes: []error{errTimeout, errTimeout},
			want:     secs(90, 90),
		},
		{
			name:     "a long streak stays at the cap",
			outcomes: slices.Repeat([]error{errTimeout}, 20),
			want:     append(secs(10, 20, 40), slices.Repeat(secs(60), 17)...),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := delaysAfter(t, tc.cfg, tc.outcomes...); !slices.Equal(got, tc.want) {
				t.Fatalf("armed %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCustomClassifier(t *testing.T) {
	t.Parallel()
	errPartial := errors.New("one endpoint throttled")
	classify := func(err error) (async.Outcome, time.Duration) {
		if errors.Is(err, errPartial) {
			return async.Throttled, 0
		}
		return async.Classify(err)
	}

	got := delaysAfter(t, async.Config{Classify: classify}, errPartial)
	if want := secs(10); !slices.Equal(got, want) {
		t.Fatalf("armed %v, want %v", got, want)
	}
}

func TestHintAndStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		cfg      async.Config
		disable  bool
		inactive bool
		outcomes []error
		hint     string
		status   string
	}{
		{name: "steady", hint: "watch 5s"},
		{name: "slow base", cfg: async.Config{Interval: 15 * time.Second}, hint: "watch 15s"},
		{name: "sub-second base", cfg: async.Config{Interval: 500 * time.Millisecond}, hint: "watch 500ms"},
		{name: "backed off", outcomes: []error{errTimeout, errTimeout}, hint: "watch 20s (slow API)", status: "refresh 20s (slow API)"},
		{name: "recovered", outcomes: []error{errTimeout, nil}, hint: "watch 5s"},
		{name: "throttled", outcomes: []error{errThrottled}, hint: "watch 10s (throttled)", status: "refresh 10s (throttled)"},
		{name: "retry-after", outcomes: []error{apierrors.NewTooManyRequests("slow down", 45)}, hint: "watch 45s (throttled)", status: "refresh 45s (throttled)"},
		{name: "paused", disable: true, outcomes: []error{errTimeout}, hint: "paused"},
		{name: "inactive", inactive: true, outcomes: []error{errTimeout}, hint: "paused"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, clk := newPoller(tc.cfg)
			p, _ = p.Begin()
			p = answer(t, p, clk, tc.outcomes...)
			if tc.disable {
				p, _ = p.WithEnabled(false)
			}
			if tc.inactive {
				p, _ = p.WithActive(false)
			}
			if p.Hint() != tc.hint || p.Status() != tc.status {
				t.Fatalf("Hint, Status = %q, %q, want %q, %q", p.Hint(), p.Status(), tc.hint, tc.status)
			}
			if !tc.disable && !tc.inactive && p.BackedOff() != (tc.status != "") {
				t.Fatalf("BackedOff = %v with status %q", p.BackedOff(), tc.status)
			}
		})
	}
}

// answer replies to one poll per outcome (one success when none are given),
// lets each armed tick start the next poll, and stops with the last reply's
// tick armed.
func answer(t *testing.T, p async.Poller, clk *asynctest.Clock, outcomes ...error) async.Poller {
	t.Helper()
	if len(outcomes) == 0 {
		outcomes = []error{nil}
	}
	last := len(outcomes) - 1
	for _, err := range outcomes[:last] {
		p = cycle(t, p, clk, err)
	}
	return poll(t, p, outcomes[last])
}
