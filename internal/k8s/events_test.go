package k8s

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

// eventsNow is the clock of these tests, the one ago counts back from.
var eventsNow = podNow

// event is a core/v1 event about pod/web-1 in demo.
func event(name, typ, reason string, count int32, first, last metav1.Time) corev1.Event {
	return corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "demo"},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: "v1", Kind: "Pod", Namespace: "demo", Name: "web-1",
		},
		Type: typ, Reason: reason, Message: "  " + reason + " happened\n",
		Count: count, FirstTimestamp: first, LastTimestamp: last,
	}
}

// TestLastSeenAsKubectlEvents pins the LAST SEEN cell of kubectl events:
// the first time of a single event, the last time with the repeat count
// and span of a repeated one.
func TestLastSeenAsKubectlEvents(t *testing.T) {
	t.Parallel()

	series := event("series", "Warning", "BackOff", 0, metav1.Time{}, metav1.Time{})
	series.EventTime = metav1.NewMicroTime(eventsNow.Add(-2 * time.Hour))
	series.Series = &corev1.EventSeries{Count: 7, LastObservedTime: metav1.NewMicroTime(eventsNow.Add(-30 * time.Second))}
	micro := event("micro", "Normal", "Scheduled", 0, metav1.Time{}, metav1.Time{})
	micro.EventTime = metav1.NewMicroTime(eventsNow.Add(-90 * time.Second))

	cases := []struct {
		name string
		in   corev1.Event
		want string
		last time.Time
	}{
		{name: "single", in: event("one", "Normal", "Pulled", 1, ago(5*time.Minute), ago(5*time.Minute)), want: "5m", last: eventsNow.Add(-5 * time.Minute)},
		{name: "repeated", in: event("many", "Warning", "BackOff", 294, ago(15*time.Hour), ago(51*time.Second)), want: "51s (x294 over 15h)", last: eventsNow.Add(-51 * time.Second)},
		{name: "series", in: series, want: "30s (x7 over 120m)", last: eventsNow.Add(-30 * time.Second)},
		{name: "event time only", in: micro, want: "90s", last: eventsNow.Add(-90 * time.Second)},
		{name: "no time at all", in: event("none", "Normal", "Odd", 0, metav1.Time{}, metav1.Time{}), want: "<unknown>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := toEvent(&tc.in, eventsNow)
			if got.LastSeen != tc.want {
				t.Errorf("LastSeen = %q, want %q", got.LastSeen, tc.want)
			}
			if !got.Last.Equal(tc.last) {
				t.Errorf("Last = %v, want %v", got.Last, tc.last)
			}
		})
	}
}

func TestToEventKeepsWhatKubectlEventsPrints(t *testing.T) {
	t.Parallel()

	in := event("web-1.17a", "Warning", "BackOff", 3, ago(time.Hour), ago(time.Minute))
	got := toEvent(&in, eventsNow)
	want := Event{
		Namespace: "demo", Name: "web-1.17a",
		Type: "Warning", Reason: "BackOff", Message: "BackOff happened",
		Object:   EventObject{APIVersion: "v1", Kind: "Pod", Namespace: "demo", Name: "web-1"},
		LastSeen: "60s (x3 over 60m)", Last: eventsNow.Add(-time.Minute),
	}
	if got != want {
		t.Errorf("toEvent =\n%+v\nwant\n%+v", got, want)
	}
}

// TestListEventsNewestFirst: kubectl events prints oldest first; k4s puts
// the newest where the cursor starts. Ties keep a fixed order.
func TestListEventsNewestFirst(t *testing.T) {
	t.Parallel()

	objs := []runtime.Object{
		ptrTo(event("b-old", "Normal", "Pulled", 1, ago(time.Hour), ago(time.Hour))),
		ptrTo(event("a-new", "Warning", "BackOff", 2, ago(time.Hour), ago(time.Second))),
		ptrTo(event("c-tie", "Normal", "Created", 1, ago(time.Minute), ago(time.Minute))),
		ptrTo(event("a-tie", "Normal", "Started", 1, ago(time.Minute), ago(time.Minute))),
	}
	c := &Client{Clientset: fake.NewSimpleClientset(objs...), Context: "test"}
	got, err := c.ListEvents(context.Background(), "demo")
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	names := make([]string, 0, len(got.Items))
	for _, e := range got.Items {
		names = append(names, e.Name)
	}
	if want := []string{"a-new", "a-tie", "c-tie", "b-old"}; !slices.Equal(names, want) {
		t.Errorf("order = %q, want %q", names, want)
	}
	if got.Cut {
		t.Error("a short list is marked cut")
	}
}

func ptrTo(e corev1.Event) *corev1.Event { return &e }

// pagedEvents serves total events in pages of the requested Limit, as an
// apiserver does for a consistent list, and records every request. item
// makes the i-th event in key order; nil serves e-00000, e-00001, ...
// about pod/web-1, each a second older than the one before. whole answers
// a list from the watch cache (ResourceVersion "0") with every event at
// once, whatever its Limit, as the cache does.
type pagedEvents struct {
	total int
	item  func(i int) corev1.Event
	whole bool

	mu   sync.Mutex
	seen []metav1.ListOptions
}

func (p *pagedEvents) at(i int) corev1.Event {
	if p.item != nil {
		return p.item(i)
	}
	return event(fmt.Sprintf("e-%05d", i), "Normal", "Pulled", 1, ago(time.Duration(i)*time.Second), ago(time.Duration(i)*time.Second))
}

func (p *pagedEvents) react(a ktesting.Action) (bool, runtime.Object, error) {
	la, ok := a.(interface{ GetListOptions() metav1.ListOptions })
	if !ok {
		return true, nil, fmt.Errorf("list events is %T", a)
	}
	o := la.GetListOptions()
	p.mu.Lock()
	p.seen = append(p.seen, o)
	p.mu.Unlock()
	start := 0
	if o.Continue != "" {
		if _, err := fmt.Sscanf(o.Continue, "from-%d", &start); err != nil {
			return true, nil, fmt.Errorf("bad continue %q", o.Continue)
		}
	}
	end := min(start+int(o.Limit), p.total)
	if p.whole && o.ResourceVersion == "0" {
		end = p.total
	}
	list := &corev1.EventList{}
	for i := start; i < end; i++ {
		list.Items = append(list.Items, p.at(i))
	}
	if end < p.total {
		list.Continue = fmt.Sprintf("from-%d", end)
	}
	return true, list, nil
}

func (p *pagedEvents) requests() []metav1.ListOptions {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.seen)
}

// TestListEventsReadsInPagesAndStops: every request carries a Limit, a
// continued one carries no resource version of its own, and reading goes
// on past MaxEvents to find the newest, stopping at maxEventsRead with the
// list marked cut.
func TestListEventsReadsInPagesAndStops(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		total    int
		opts     []ListOption
		wantRead int
		wantCut  bool
		wantReqs int
	}{
		{name: "one page", total: 3, wantRead: 3, wantReqs: 1},
		{name: "several pages from the cache", total: 1200, opts: []ListOption{FromCache()}, wantRead: 1200, wantReqs: 3},
		{name: "more than MaxEvents", total: MaxEvents + 700, wantRead: MaxEvents, wantCut: true, wantReqs: (MaxEvents + 700 + eventsPage - 1) / eventsPage},
		{name: "more than maxEventsRead", total: maxEventsRead + 300, wantRead: MaxEvents, wantCut: true, wantReqs: maxEventsRead / eventsPage},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pages := &pagedEvents{total: tc.total}
			cs := fake.NewSimpleClientset()
			cs.PrependReactor("list", "events", pages.react)
			c := &Client{Clientset: cs, Context: "test"}
			got, err := c.ListEvents(context.Background(), "demo", tc.opts...)
			if err != nil {
				t.Fatalf("ListEvents: %v", err)
			}
			if len(got.Items) != tc.wantRead || got.Cut != tc.wantCut {
				t.Errorf("read %d (cut %v), want %d (cut %v)", len(got.Items), got.Cut, tc.wantRead, tc.wantCut)
			}
			reqs := pages.requests()
			if len(reqs) != tc.wantReqs {
				t.Fatalf("requests = %d, want %d", len(reqs), tc.wantReqs)
			}
			for i, o := range reqs {
				if o.Limit != eventsPage {
					t.Errorf("request %d Limit = %d, want %d", i, o.Limit, eventsPage)
				}
				if i > 0 && o.ResourceVersion != "" {
					t.Errorf("continued request %d sets resource version %q", i, o.ResourceVersion)
				}
			}
			if len(tc.opts) > 0 && reqs[0].ResourceVersion != "0" {
				t.Errorf("first request from the cache has resource version %q", reqs[0].ResourceVersion)
			}
		})
	}
}

// TestListEventsPollAndRefreshAgree: a poll from the watch cache gets
// every event in one answer, a refresh reads them in pages; both keep the
// same newest MaxEvents. The newest events come last in key order, so a
// read that stopped at MaxEvents would keep the oldest.
func TestListEventsPollAndRefreshAgree(t *testing.T) {
	t.Parallel()

	const total = MaxEvents + 1500
	newestLast := func(i int) corev1.Event {
		at := ago(time.Duration(total-i) * time.Second)
		return event(fmt.Sprintf("e-%05d", i), "Normal", "Pulled", 1, at, at)
	}
	read := func(opts ...ListOption) EventList {
		t.Helper()
		cs := fake.NewSimpleClientset()
		cs.PrependReactor("list", "events", (&pagedEvents{total: total, item: newestLast, whole: true}).react)
		c := &Client{Clientset: cs, Context: "test"}
		got, err := c.ListEvents(context.Background(), "", opts...)
		if err != nil {
			t.Fatalf("ListEvents: %v", err)
		}
		return got
	}
	poll, refresh := read(FromCache()), read()
	if !poll.Cut || !refresh.Cut || len(poll.Items) != MaxEvents || len(refresh.Items) != MaxEvents {
		t.Fatalf("poll kept %d (cut %v), refresh %d (cut %v); want %d cut", len(poll.Items), poll.Cut, len(refresh.Items), refresh.Cut, MaxEvents)
	}
	for i := range poll.Items {
		if poll.Items[i].Name != refresh.Items[i].Name {
			t.Fatalf("row %d: poll %s, refresh %s", i, poll.Items[i].Name, refresh.Items[i].Name)
		}
	}
	if first, last := refresh.Items[0].Name, refresh.Items[MaxEvents-1].Name; first != fmt.Sprintf("e-%05d", total-1) || last != "e-01500" {
		t.Errorf("kept %s … %s, want the newest", first, last)
	}
}

// TestListEventsKeepsTheNewestWhenTheServerIgnoresTheLimit: a list from
// the watch cache can come back whole whatever Limit says; only the
// newest MaxEvents are kept.
func TestListEventsKeepsTheNewestWhenTheServerIgnoresTheLimit(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
		list := &corev1.EventList{}
		for i := range MaxEvents + 5 {
			// e-00000 is the oldest.
			at := ago(time.Duration(MaxEvents+5-i) * time.Second)
			list.Items = append(list.Items, event(fmt.Sprintf("e-%05d", i), "Normal", "Pulled", 1, at, at))
		}
		return true, list, nil
	})
	c := &Client{Clientset: cs, Context: "test"}
	got, err := c.ListEvents(context.Background(), "", FromCache())
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(got.Items) != MaxEvents || !got.Cut {
		t.Fatalf("kept %d (cut %v), want %d cut", len(got.Items), got.Cut, MaxEvents)
	}
	if first, last := got.Items[0].Name, got.Items[len(got.Items)-1].Name; first != fmt.Sprintf("e-%05d", MaxEvents+4) || last != "e-00005" {
		t.Errorf("kept %s … %s, want the newest", first, last)
	}
}

func TestListEventsWrapsTheError(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()
	cs.PrependReactor("list", "events", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("boom")
	})
	c := &Client{Clientset: cs, Context: "test"}
	if _, err := c.ListEvents(context.Background(), "demo"); err == nil || err.Error() != "list events: boom" {
		t.Errorf("err = %v, want list events: boom", err)
	}
}
