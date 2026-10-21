package k8s

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// eventsPage is the Limit of one events request, the chunk kubectl
	// events reads in.
	eventsPage = 500
	// MaxEvents bounds what ListEvents keeps: a busy namespace can hold
	// tens of thousands of events, polled every few seconds.
	MaxEvents = 10000
)

// Event is one core/v1 Event, with what `kubectl events` prints of it.
// Its strings are as the API holds them: whoever may create events wrote
// them, so they are made inert before they are drawn.
type Event struct {
	Namespace string
	Name      string
	Type      string
	Reason    string
	// Message is trimmed of surrounding blanks, as kubectl trims it.
	Message string
	Object  EventObject
	// LastSeen is kubectl events' LAST SEEN cell at list time: "5m", or
	// "51s (x294 over 15h)" for an event seen more than once.
	LastSeen string
	// Last is when the event last happened, the time kubectl sorts by;
	// zero when the event names none.
	Last time.Time
}

// EventObject is the object an event is about, its involvedObject.
type EventObject struct {
	APIVersion string
	Kind       string
	Namespace  string
	Name       string
}

// EventList is the events of a namespace, or of all of them, newest
// first.
type EventList struct {
	Items []Event
	// Cut is set when there were more than MaxEvents: reading stopped
	// there, or only the newest MaxEvents of one whole answer were kept.
	Cut bool
}

// ListEvents lists the events in namespace, or in all namespaces when
// namespace is "", newest first: `kubectl events` prints the same events
// oldest first. It reads in pages of eventsPage and stops at MaxEvents.
// opts apply to the first page; a continued page takes its resource
// version from the continue token, as the API requires.
func (c *Client) ListEvents(ctx context.Context, namespace string, opts ...ListOption) (EventList, error) {
	o := listOptions(opts)
	o.Limit = eventsPage
	var raw []corev1.Event
	for {
		list, err := c.Clientset.CoreV1().Events(namespace).List(ctx, o)
		if err != nil {
			return EventList{}, fmt.Errorf("list events: %w", err)
		}
		raw = append(raw, list.Items...)
		if list.Continue == "" || len(raw) >= MaxEvents {
			return newestEvents(raw, list.Continue != "", time.Now()), nil
		}
		o = metav1.ListOptions{Limit: eventsPage, Continue: list.Continue}
	}
}

// newestEvents converts raw at now and keeps the newest MaxEvents; more
// marks a list the server had not finished.
func newestEvents(raw []corev1.Event, more bool, now time.Time) EventList {
	items := make([]Event, 0, len(raw))
	for i := range raw {
		items = append(items, toEvent(&raw[i], now))
	}
	slices.SortStableFunc(items, newestFirst)
	if len(items) > MaxEvents {
		return EventList{Items: slices.Clip(items[:MaxEvents]), Cut: true}
	}
	return EventList{Items: items, Cut: more}
}

// newestFirst orders events by when they last happened, newest first,
// then by namespace and name so that equal times keep their rows.
func newestFirst(a, b Event) int {
	if c := b.Last.Compare(a.Last); c != 0 {
		return c
	}
	return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
}

func toEvent(e *corev1.Event, now time.Time) Event {
	o := e.InvolvedObject
	return Event{
		Namespace: e.Namespace,
		Name:      e.Name,
		Type:      e.Type,
		Reason:    e.Reason,
		Message:   strings.TrimSpace(e.Message),
		Object:    EventObject{APIVersion: o.APIVersion, Kind: o.Kind, Namespace: o.Namespace, Name: o.Name},
		LastSeen:  eventAge(*e, now),
		Last:      eventTime(*e),
	}
}
