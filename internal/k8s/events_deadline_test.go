package k8s

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes/fake"
)

// TestListEventsReturnsWhatItReadAtTheDeadline: a refresh of a busy
// cluster reads up to maxEventsRead/eventsPage pages and can run out of
// time between them. The pages read by then come back, cut, instead of a
// timeout the poller would back off on; no page is started within
// readReserve of the deadline. A first page that times out, or a page
// that fails for another reason, is still an error.
func TestListEventsReturnsWhatItReadAtTheDeadline(t *testing.T) {
	t.Parallel()

	timedOut := fmt.Errorf("get events: %w", context.DeadlineExceeded)
	cases := []struct {
		name string
		// budget is the deadline the read gets; 0 is none.
		budget   time.Duration
		fail     map[int]error
		wantRead int
		wantCut  bool
		wantReqs int
		wantErr  error
	}{
		{name: "time enough for every page", budget: time.Minute, wantRead: 5 * eventsPage, wantReqs: 5},
		{name: "no time left for another page", budget: readReserve / 2, wantRead: eventsPage, wantCut: true, wantReqs: 1},
		{name: "a continued page times out", fail: map[int]error{2: timedOut}, wantRead: 2 * eventsPage, wantCut: true, wantReqs: 3},
		{name: "the first page times out", fail: map[int]error{0: timedOut}, wantReqs: 1, wantErr: context.DeadlineExceeded},
		{name: "a continued page fails", fail: map[int]error{1: errBoom}, wantReqs: 2, wantErr: errBoom},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pages := &pagedEvents{total: 5 * eventsPage, fail: tc.fail}
			cs := fake.NewSimpleClientset()
			cs.PrependReactor("list", "events", pages.react)
			c := &Client{Clientset: cs, Context: "test"}
			ctx := context.Background()
			if tc.budget > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, tc.budget)
				defer cancel()
			}
			got, err := c.ListEvents(ctx, "", eventsNow)
			if reqs := len(pages.requests()); reqs != tc.wantReqs {
				t.Errorf("requests = %d, want %d", reqs, tc.wantReqs)
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ListEvents: %v", err)
			}
			if len(got.Items) != tc.wantRead || got.Cut != tc.wantCut {
				t.Errorf("read %d (cut %v), want %d (cut %v)", len(got.Items), got.Cut, tc.wantRead, tc.wantCut)
			}
		})
	}
}

var errBoom = errors.New("boom")
