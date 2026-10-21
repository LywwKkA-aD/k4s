package k8s

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
)

// eventSubject names the object whose events a describe lists.
type eventSubject struct {
	kind, namespace, name string
	uid                   types.UID
}

// eventList is the events about one object, or why they could not be
// read. Events are best effort: a failed list never fails the describe.
type eventList struct {
	items []corev1.Event
	err   error
}

// eventsFor lists the events about s. They are matched by uid, as kubectl
// describe matches them, so an earlier object of the same name lends none
// of its own; without a uid, by kind and name. The field selector only
// narrows what the server sends.
func (c *Client) eventsFor(ctx context.Context, s eventSubject) eventList {
	sel := fields.Set{"involvedObject.name": s.name, "involvedObject.namespace": s.namespace}
	if s.uid != "" {
		sel["involvedObject.uid"] = string(s.uid)
	}
	list, err := c.Clientset.CoreV1().Events(s.namespace).List(ctx, metav1.ListOptions{FieldSelector: sel.AsSelector().String()})
	if err != nil {
		return eventList{err: fmt.Errorf("list events: %w", err)}
	}
	mine := make([]corev1.Event, 0, len(list.Items))
	for _, e := range list.Items {
		if s.about(e.InvolvedObject) {
			mine = append(mine, e)
		}
	}
	return eventList{items: mine}
}

func (s eventSubject) about(ref corev1.ObjectReference) bool {
	if s.uid != "" {
		return ref.UID == s.uid
	}
	return ref.Kind == s.kind && ref.Name == s.name
}

// writeEvents writes the events oldest first, as kubectl does, with the
// repeat count of a series. No events is "<none>"; events that could not
// be read are "<unavailable: forbidden>" and the like, never the raw
// error, which can name the user.
func writeEvents(w describeWriter, ev eventList, now time.Time) {
	switch {
	case ev.err != nil:
		w.write(0, "Events:\t<unavailable: %s>\n", unavailable(ev.err))
		return
	case len(ev.items) == 0:
		w.write(0, "Events:\t<none>\n")
		return
	}
	w.flush()
	sorted := slices.Clone(ev.items)
	slices.SortStableFunc(sorted, func(a, b corev1.Event) int { return eventTime(a).Compare(eventTime(b)) })
	w.write(0, "Events:\n  Type\tReason\tAge\tFrom\tMessage\n")
	w.write(1, "----\t------\t----\t----\t-------\n")
	for _, e := range sorted {
		w.write(1, "%s\t%s\t%s\t%s\t%s\n", e.Type, e.Reason, eventAge(e, now), eventSource(e), strings.TrimSpace(e.Message))
	}
}

// unavailable is the short reason events could not be listed.
func unavailable(err error) string {
	switch {
	case apierrors.IsForbidden(err):
		return "forbidden"
	case apierrors.IsUnauthorized(err):
		return "unauthorized"
	case errors.Is(err, context.DeadlineExceeded), apierrors.IsTimeout(err), apierrors.IsServerTimeout(err):
		return "timeout"
	}
	return "error"
}

// eventTime is when the event last happened, the time kubectl sorts by.
func eventTime(e corev1.Event) time.Time {
	switch {
	case e.Series != nil:
		return e.Series.LastObservedTime.Time
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Time
	}
	return e.EventTime.Time
}

// eventAge is kubectl's AGE cell: "51s", or "51s (x294 over 15h)" for an
// event seen more than once.
func eventAge(e corev1.Event, now time.Time) string {
	first := since(e.FirstTimestamp, now)
	if !e.EventTime.IsZero() {
		first = since(metav1.NewTime(e.EventTime.Time), now)
	}
	switch {
	case e.Series != nil:
		return fmt.Sprintf("%s (x%d over %s)", since(metav1.NewTime(e.Series.LastObservedTime.Time), now), e.Series.Count, first)
	case e.Count > 1:
		return fmt.Sprintf("%s (x%d over %s)", since(e.LastTimestamp, now), e.Count, first)
	}
	return first
}

func eventSource(e corev1.Event) string {
	if e.Source.Component != "" {
		return e.Source.Component
	}
	return e.ReportingController
}
