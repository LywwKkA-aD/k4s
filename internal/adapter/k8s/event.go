package k8s

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// EventFilterOptions specifies filters for event retrieval
type EventFilterOptions struct {
	Type       string // "Normal", "Warning", or "" for all
	ObjectKind string // "Pod", "Deployment", etc. or "" for all
	Limit      int64  // Maximum number of events (0 = no limit)
}

// GetEvents returns all events in the specified namespace
func (c *Client) GetEvents(ctx context.Context, namespace string) ([]domain.Event, error) {
	return c.GetEventsFiltered(ctx, namespace, EventFilterOptions{})
}

// GetEventsFiltered returns events with optional filtering
func (c *Client) GetEventsFiltered(ctx context.Context, namespace string, opts EventFilterOptions) ([]domain.Event, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	listOpts := metav1.ListOptions{}
	if opts.Limit > 0 {
		listOpts.Limit = opts.Limit
	}

	// Build field selector for filtering
	var fieldSelectors []string
	if opts.Type != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("type=%s", opts.Type))
	}
	if opts.ObjectKind != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("involvedObject.kind=%s", opts.ObjectKind))
	}
	if len(fieldSelectors) > 0 {
		listOpts.FieldSelector = joinSelectors(fieldSelectors)
	}

	eventList, err := c.clientset.CoreV1().Events(namespace).List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}

	events := make([]domain.Event, 0, len(eventList.Items))
	for _, e := range eventList.Items {
		events = append(events, convertEvent(&e))
	}

	// Sort chronologically (oldest first, newest last/at bottom for follow mode)
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastSeenTime.Before(events[j].LastSeenTime)
	})

	return events, nil
}

// GetAllNamespaceEvents returns events from all namespaces
func (c *Client) GetAllNamespaceEvents(ctx context.Context, opts EventFilterOptions) ([]domain.Event, error) {
	listOpts := metav1.ListOptions{}
	if opts.Limit > 0 {
		listOpts.Limit = opts.Limit
	}

	// Build field selector for filtering
	var fieldSelectors []string
	if opts.Type != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("type=%s", opts.Type))
	}
	if opts.ObjectKind != "" {
		fieldSelectors = append(fieldSelectors, fmt.Sprintf("involvedObject.kind=%s", opts.ObjectKind))
	}
	if len(fieldSelectors) > 0 {
		listOpts.FieldSelector = joinSelectors(fieldSelectors)
	}

	// Use empty namespace to get events from all namespaces
	eventList, err := c.clientset.CoreV1().Events("").List(ctx, listOpts)
	if err != nil {
		return nil, fmt.Errorf("list all events: %w", err)
	}

	events := make([]domain.Event, 0, len(eventList.Items))
	for _, e := range eventList.Items {
		events = append(events, convertEvent(&e))
	}

	// Sort chronologically (oldest first, newest last/at bottom for follow mode)
	sort.Slice(events, func(i, j int) bool {
		return events[i].LastSeenTime.Before(events[j].LastSeenTime)
	})

	return events, nil
}

func convertEvent(e *corev1.Event) domain.Event {
	objectRef := fmt.Sprintf("%s/%s", e.InvolvedObject.Kind, e.InvolvedObject.Name)

	// Use EventTime if available (newer events API), otherwise fall back to timestamps
	lastSeenTime := e.LastTimestamp.Time
	if !e.EventTime.IsZero() {
		lastSeenTime = e.EventTime.Time
	}

	firstSeen := formatAge(e.FirstTimestamp.Time)
	lastSeen := formatAge(lastSeenTime)

	return domain.Event{
		Name:            e.Name,
		Namespace:       e.Namespace,
		Type:            e.Type,
		Reason:          e.Reason,
		Message:         e.Message,
		Object:          objectRef,
		ObjectKind:      e.InvolvedObject.Kind,
		ObjectName:      e.InvolvedObject.Name,
		Count:           e.Count,
		FirstSeen:       firstSeen,
		LastSeen:        lastSeen,
		Age:             lastSeen,
		SourceComponent: e.Source.Component,
		LastSeenTime:    lastSeenTime,
	}
}

// joinSelectors joins field selectors with comma
func joinSelectors(selectors []string) string {
	result := ""
	for i, s := range selectors {
		if i > 0 {
			result += ","
		}
		result += s
	}
	return result
}
