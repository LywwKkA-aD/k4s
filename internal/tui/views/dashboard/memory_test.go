package dashboard_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/LywwKkA-aD/k4s/internal/k8s/dash"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
	"github.com/LywwKkA-aD/k4s/internal/tui/views/dashboard"
)

// TestMemoryShowsDataAtOnce is the visit the root makes: the dashboard is
// left for another view and built again from its Memory. Before its first
// new answer it already shows the data, the history and the cursor.
func TestMemoryShowsDataAtOnce(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	f.vary = true
	c := tuitest.NewFakeClient(t)
	m := mountOn(t, c, f, wideW, wideH)
	m = advance(t, m, f, 2*time.Minute)
	m, _ = press(t, m, "3", "down")
	left := tuitest.Screen(m)

	back := dashboard.New(c, nil, append(f.options(), dashboard.WithMemory(m.Memory()))...)
	back, _ = tuitest.Drive(t, back, tuitest.Resize(wideW, wideH))
	if got := tuitest.Screen(back); got != left {
		t.Fatalf("the dashboard came back different:\n%s\n--- left ---\n%s", got, left)
	}
	if got := command(back); got != firstPod {
		t.Fatalf("cursor after the visit = %q, want %q", got, firstPod)
	}

	// Time away shows as blank history, and only what is due is fetched.
	f.clock.Advance(10 * time.Minute)
	calls := len(f.calls())
	_, _ = tuitest.Drive(t, back, back.Init()())
	plans := f.calls()
	if len(plans) != calls+1 || plans[len(plans)-1].Force {
		t.Fatalf("coming back ran %d refreshes, want one plain one", len(plans)-calls)
	}
}

// TestMemoryKeepsFocusWhenTheReplyComesFirst: the root starts the view and
// sizes it in one batch, so the first reply can land before the size.
func TestMemoryKeepsFocusWhenTheReplyComesFirst(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	c := tuitest.NewFakeClient(t)
	m, _ := press(t, mountOn(t, c, f, wideW, wideH), "3", "down")
	back := dashboard.New(c, nil, append(f.options(), dashboard.WithMemory(m.Memory()))...)
	back, _ = tuitest.Drive(t, back, back.Init()(), tuitest.Resize(wideW, wideH))
	if got := command(back); got != firstPod {
		t.Fatalf("cursor = %q, want %q", got, firstPod)
	}
}

func TestMemoryOfAnotherClientIsDropped(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	m, _ := press(t, mount(t, f, wideW, wideH), "3", "down")
	other := dashboard.New(tuitest.NewFakeClient(t), nil, dashboard.WithMemory(m.Memory()))
	other, _ = tuitest.Drive(t, other, tuitest.Resize(wideW, wideH))
	if got := command(other); got != verdictCmd {
		t.Fatalf("command on another client = %q, want the banner's", got)
	}
	if !strings.Contains(tuitest.Screen(other), "loading") {
		t.Fatalf("another client must start empty:\n%s", tuitest.Screen(other))
	}
}

func TestMemoryRescopesForAnotherNamespace(t *testing.T) {
	t.Parallel()

	f := newFeed(busyK3s)
	c := tuitest.NewFakeClient(t)
	m, _ := press(t, mountOn(t, c, f, wideW, wideH), "3", "down")
	shop := dashboard.New(c, nil, append(f.options(), dashboard.WithMemory(m.Memory()), dashboard.WithNamespace("shop"))...)
	shop, _ = tuitest.Mount(t, shop, wideW, wideH)
	plans := f.calls()
	if got := plans[len(plans)-1].Scope.Namespace; got != "shop" {
		t.Fatalf("the rescoped dashboard listed %q, want shop", got)
	}
	if got := command(shop); got != podsCmd {
		t.Fatalf("command after :ns = %q, want the pods panel without its row", got)
	}
}

// TestCursorFollowsItsObject is the critique's L5/L6 case for the
// dashboard: a poll that re-sorts the rows keeps the cursor on the pod the
// user picked, and a pod that is gone moves it to its neighbour with a
// status line.
func TestCursorFollowsItsObject(t *testing.T) {
	t.Parallel()

	reorder := func(drop bool) func() dash.Snapshot {
		return func() dash.Snapshot {
			s := busyK3s()
			s.Pods.Problems = slices.Clone(s.Pods.Problems)
			slices.Reverse(s.Pods.Problems)
			if drop {
				s.Pods.Problems = slices.DeleteFunc(s.Pods.Problems, func(r dash.PodRow) bool {
					return r.Key.Name == "payments-worker-6c9f7d8b5-xk2lp"
				})
			}
			return s
		}
	}

	f := newFeed(busyK3s)
	m, _ := press(t, mount(t, f, wideW, wideH), "3", "down")
	f.set(reorder(false))
	m = advance(t, m, f, 5*time.Second)
	if got := command(m); got != firstPod {
		t.Fatalf("after a re-sort the cursor is on %q, want %q", got, firstPod)
	}

	f.set(reorder(true))
	m, out := tuitest.Drive(t, m, f.clock.Advance(5*time.Second)...)
	msg, ok := tuitest.Find[views.ToastMsg](out)
	if !ok || msg.Text != "pod shop/payments-worker-6c9f7d8b5-xk2lp is gone" {
		t.Fatalf("a vanished row should be named:\n%s", tuitest.Describe(out))
	}
	if got := command(m); !strings.HasPrefix(got, "kubectl describe pod ") || got == firstPod {
		t.Fatalf("the cursor did not move to a neighbour: %q", got)
	}
}
