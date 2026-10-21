package k8s

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// TestDescribeEventsAreBounded: the built-in describer asks for at most
// describeEventsLimit events and says so when the server holds more.
func TestDescribeEventsAreBounded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		next     string
		wantNote bool
	}{
		{name: "all listed"},
		{name: "more on the server", next: "next-page", wantNote: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pod := crashLoopPod()
			ev := podEvent(pod, "backoff", corev1.EventTypeWarning, "BackOff", "back-off", 3, before(time.Hour), before(time.Minute))
			cs := fake.NewSimpleClientset(pod)
			var limit atomic.Int64
			cs.PrependReactor("list", "events", func(a ktesting.Action) (bool, runtime.Object, error) {
				la, ok := a.(interface{ GetListOptions() metav1.ListOptions })
				if !ok {
					t.Errorf("list events is %T", a)
					return true, &corev1.EventList{}, nil
				}
				limit.Store(la.GetListOptions().Limit)
				return true, &corev1.EventList{ListMeta: metav1.ListMeta{Continue: tc.next}, Items: []corev1.Event{*ev}}, nil
			})
			c := &Client{Clientset: cs, Context: "test"}
			out, err := c.describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
			if err != nil {
				t.Fatalf("describePod: %v", err)
			}
			if got := limit.Load(); got != describeEventsLimit {
				t.Errorf("events list Limit = %d, want %d", got, describeEventsLimit)
			}
			if !strings.Contains(out, "back-off") {
				t.Errorf("the listed event is missing:\n%s", out)
			}
			note := "more events: kubectl events --for pod/" + pod.Name + " -n " + pod.Namespace
			if got := strings.Contains(out, note); got != tc.wantNote {
				t.Errorf("note %q shown = %v, want %v:\n%s", note, got, tc.wantNote, out)
			}
		})
	}
}
