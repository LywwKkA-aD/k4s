package k8s

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestDescribeEventsKeepTheNewest: the built-in describer reads the
// object's events in pages up to describeEventsRead, keeps the newest
// describeEventsShown of them, oldest first as kubectl prints them, and
// says how many there were. The fixture's newest events come last in key
// order, so keeping the first ones read would show the oldest.
func TestDescribeEventsKeepTheNewest(t *testing.T) {
	t.Parallel()

	const more = "kubectl events --for pod/crash-loop -n k4s-broken"
	cases := []struct {
		name        string
		total       int
		wantReqs    int
		first, last int // the oldest and the newest event shown
		note        string
	}{
		{name: "all listed", total: 3, wantReqs: 1, first: 0, last: 2},
		{name: "more than shown", total: 742, wantReqs: 2, first: 242, last: 741, note: "… showing the newest 500 of 742: " + more},
		{name: "more than read", total: 2600, wantReqs: 4, first: 1500, last: 1999, note: "… showing the newest 500 of more than 2000: " + more},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pod := crashLoopPod()
			pages := &pagedEvents{total: tc.total, item: func(i int) corev1.Event {
				at := before(time.Duration(tc.total-i) * time.Minute)
				return *podEvent(pod, fmt.Sprintf("e-%05d", i), corev1.EventTypeNormal, "Pulled", fmt.Sprintf("msg-%05d", i), 1, at, at)
			}}
			cs := fake.NewSimpleClientset(pod)
			cs.PrependReactor("list", "events", pages.react)
			c := &Client{Clientset: cs, Context: "test"}
			out, err := c.describePod(context.Background(), pod.Namespace, pod.Name, describeNow)
			if err != nil {
				t.Fatalf("describePod: %v", err)
			}
			reqs := pages.requests()
			if len(reqs) != tc.wantReqs {
				t.Fatalf("requests = %d, want %d", len(reqs), tc.wantReqs)
			}
			for i, o := range reqs {
				if o.Limit != eventsPage || o.FieldSelector != reqs[0].FieldSelector || (i > 0) != (o.Continue != "") {
					t.Errorf("request %d = %+v", i, o)
				}
			}
			shown := strings.Count(out, "msg-")
			first, last := fmt.Sprintf("msg-%05d", tc.first), fmt.Sprintf("msg-%05d", tc.last)
			if shown != tc.last-tc.first+1 || !strings.Contains(out, first) || strings.Index(out, first) > strings.Index(out, last) {
				t.Errorf("shown %d events, want %s … %s oldest first:\n%s", shown, first, last, out)
			}
			if got := strings.Contains(out, "… "); got != (tc.note != "") || !strings.Contains(out, tc.note) {
				t.Errorf("note %q (any shown %v):\n%s", tc.note, got, out)
			}
		})
	}
}

// TestDescribeEventsNoteScope: the note's kubectl events takes -n only for
// an object that has a namespace.
func TestDescribeEventsNoteScope(t *testing.T) {
	t.Parallel()

	ev := *podEvent(crashLoopPod(), "one", corev1.EventTypeNormal, "Pulled", "pulled", 1, before(time.Minute), before(time.Minute))
	cases := []struct {
		subject eventSubject
		want    string
	}{
		{subject: eventSubject{kind: "Pod", namespace: "demo", name: "web-1"}, want: "kubectl events --for pod/web-1 -n demo"},
		{subject: eventSubject{kind: "Node", name: "k3s-server"}, want: "kubectl events --for node/k3s-server\n"},
	}
	for _, tc := range cases {
		t.Run(tc.subject.kind, func(t *testing.T) {
			t.Parallel()
			out := tabbed(func(w describeWriter) {
				writeEvents(w, eventList{items: []corev1.Event{ev}, total: 600, subject: tc.subject}, describeNow)
			})
			if !strings.Contains(out, "… showing the newest 1 of 600: "+tc.want) {
				t.Errorf("note:\n%s", out)
			}
		})
	}
}
