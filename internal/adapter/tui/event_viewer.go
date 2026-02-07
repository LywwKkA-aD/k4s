package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/domain"
)

// EventViewer is the model for viewing cluster events in a log-like format
type EventViewer struct {
	events        []domain.Event
	viewport      viewport.Model
	styles        Styles
	width         int
	height        int
	ready         bool
	following     bool
	autoScroll    bool
	warningsOnly  bool
	kindFilter    string // "", "Pod", "Deployment", "Service", etc.
	searchQuery   string
	totalEvents   int
	filteredCount int
}

// NewEventViewer creates a new event viewer
func NewEventViewer(styles Styles) EventViewer {
	return EventViewer{
		styles:     styles,
		following:  true, // Default: follow mode on
		autoScroll: true,
		events:     make([]domain.Event, 0),
	}
}

// SetSize sets the viewport size
func (e *EventViewer) SetSize(width, height int) {
	e.width = width
	e.height = height
	e.viewport = viewport.New(width, height)
	e.viewport.Style = lipgloss.NewStyle()
	e.ready = true
	e.updateContent()
}

// SetEvents sets the events to display
func (e *EventViewer) SetEvents(events []domain.Event) {
	e.events = events
	e.totalEvents = len(events)
	e.updateContent()

	// Auto-scroll to bottom if following
	if e.following && e.autoScroll && e.ready {
		e.viewport.GotoBottom()
	}
}

// Clear clears the events
func (e *EventViewer) Clear() {
	e.events = make([]domain.Event, 0)
	e.totalEvents = 0
	e.filteredCount = 0
	e.updateContent()
}

// SetFollowing sets the follow mode
func (e *EventViewer) SetFollowing(following bool) {
	e.following = following
	e.autoScroll = following
}

// IsFollowing returns whether follow mode is active
func (e *EventViewer) IsFollowing() bool {
	return e.following
}

// ToggleFollowing toggles follow mode
func (e *EventViewer) ToggleFollowing() bool {
	e.following = !e.following
	e.autoScroll = e.following
	if e.following && e.ready {
		e.viewport.GotoBottom()
	}
	return e.following
}

// ToggleWarningsOnly toggles warnings-only filter
func (e *EventViewer) ToggleWarningsOnly() bool {
	e.warningsOnly = !e.warningsOnly
	e.updateContent()
	return e.warningsOnly
}

// IsWarningsOnly returns whether warnings-only filter is active
func (e *EventViewer) IsWarningsOnly() bool {
	return e.warningsOnly
}

// CycleKindFilter cycles through kind filters
func (e *EventViewer) CycleKindFilter() string {
	kinds := []string{"", "Pod", "Deployment", "ReplicaSet", "Service", "Node"}
	currentIdx := 0
	for i, k := range kinds {
		if k == e.kindFilter {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + 1) % len(kinds)
	e.kindFilter = kinds[nextIdx]
	e.updateContent()
	return e.kindFilter
}

// KindFilter returns the current kind filter
func (e *EventViewer) KindFilter() string {
	return e.kindFilter
}

// SetSearchQuery sets the search query
func (e *EventViewer) SetSearchQuery(query string) {
	e.searchQuery = query
	e.updateContent()
}

// SearchQuery returns the current search query
func (e *EventViewer) SearchQuery() string {
	return e.searchQuery
}

func (e *EventViewer) filterEvents() []domain.Event {
	if !e.warningsOnly && e.kindFilter == "" && e.searchQuery == "" {
		return e.events
	}

	filtered := make([]domain.Event, 0)
	query := strings.ToLower(e.searchQuery)

	for _, evt := range e.events {
		// Filter by warnings only
		if e.warningsOnly && evt.Type != domain.EventTypeWarning {
			continue
		}

		// Filter by kind
		if e.kindFilter != "" && evt.ObjectKind != e.kindFilter {
			continue
		}

		// Filter by search query
		if query != "" {
			searchText := strings.ToLower(evt.Message + " " + evt.Reason + " " + evt.ObjectName)
			if !strings.Contains(searchText, query) {
				continue
			}
		}

		filtered = append(filtered, evt)
	}

	return filtered
}

func (e *EventViewer) updateContent() {
	if !e.ready {
		return
	}

	filtered := e.filterEvents()
	e.filteredCount = len(filtered)

	if len(filtered) == 0 {
		msg := "No events"
		if e.warningsOnly {
			msg = "No warning events"
		}
		if e.kindFilter != "" {
			msg += fmt.Sprintf(" for %s", e.kindFilter)
		}
		e.viewport.SetContent(msg)
		return
	}

	var sb strings.Builder

	for i, evt := range filtered {
		line := e.formatEvent(evt)
		sb.WriteString(line)
		if i < len(filtered)-1 {
			sb.WriteString("\n")
		}
	}

	e.viewport.SetContent(sb.String())
}

func (e *EventViewer) formatEvent(evt domain.Event) string {
	// Format: [TIME] TYPE REASON OBJECT MESSAGE
	// Use colors based on event type

	var typeStyle lipgloss.Style
	if evt.Type == domain.EventTypeWarning {
		typeStyle = lipgloss.NewStyle().Foreground(colorError).Bold(true)
	} else {
		typeStyle = lipgloss.NewStyle().Foreground(colorMuted)
	}

	reasonStyle := lipgloss.NewStyle().Foreground(colorSecondary)
	objectStyle := lipgloss.NewStyle().Foreground(colorPrimary)
	messageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))
	timeStyle := lipgloss.NewStyle().Foreground(colorMuted)

	// Format timestamp - use Age which is already formatted nicely
	timeStr := evt.Age
	if timeStr == "" {
		timeStr = evt.LastSeen
	}

	// Build the line
	var line strings.Builder

	// Time
	line.WriteString(timeStyle.Render(fmt.Sprintf("[%s] ", timeStr)))

	// Type (padded)
	line.WriteString(typeStyle.Render(fmt.Sprintf("%-7s ", evt.Type)))

	// Reason (padded)
	line.WriteString(reasonStyle.Render(fmt.Sprintf("%-25s ", truncateString(evt.Reason, 25))))

	// Object (kind/name)
	objectStr := fmt.Sprintf("%s/%s", evt.ObjectKind, evt.ObjectName)
	line.WriteString(objectStyle.Render(fmt.Sprintf("%-40s ", truncateString(objectStr, 40))))

	// Message (rest of line)
	maxMsgLen := max(e.width-90, 20) // Approximate space for other columns
	msg := truncateString(evt.Message, maxMsgLen)

	// Highlight message for warnings
	if evt.Type == domain.EventTypeWarning {
		line.WriteString(typeStyle.Render(msg))
	} else {
		line.WriteString(messageStyle.Render(msg))
	}

	// Count indicator if multiple
	if evt.Count > 1 {
		countStyle := lipgloss.NewStyle().Foreground(colorWarning)
		line.WriteString(countStyle.Render(fmt.Sprintf(" (x%d)", evt.Count)))
	}

	return line.String()
}

// Update handles messages
func (e *EventViewer) Update(msg tea.Msg) (EventViewer, tea.Cmd) {
	// Disable auto-scroll on manual scroll
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "down", "pgup", "pgdown", "k", "j":
			e.autoScroll = false
			e.following = false
		case "G":
			// Go to bottom re-enables auto-scroll and follow
			e.autoScroll = true
			e.following = true
			e.viewport.GotoBottom()
			return *e, nil
		case "g":
			// Go to top disables auto-scroll and follow
			e.autoScroll = false
			e.following = false
			e.viewport.GotoTop()
			return *e, nil
		}
	}

	var cmd tea.Cmd
	e.viewport, cmd = e.viewport.Update(msg)
	return *e, cmd
}

// View renders the event viewer
func (e *EventViewer) View() string {
	if !e.ready {
		return "Loading..."
	}
	return e.viewport.View()
}

// ScrollPercent returns the scroll percentage
func (e *EventViewer) ScrollPercent() float64 {
	return e.viewport.ScrollPercent()
}

// TotalEvents returns the total number of events
func (e *EventViewer) TotalEvents() int {
	return e.totalEvents
}

// FilteredCount returns the number of filtered events
func (e *EventViewer) FilteredCount() int {
	return e.filteredCount
}

// RenderHeader returns the event viewer header
func (e *EventViewer) RenderHeader() string {
	title := "Events"

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary)

	// Status indicators
	var indicators []string

	// Follow indicator
	if e.following {
		indicator := lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Following")
		indicators = append(indicators, indicator+" "+label)
	}

	// Warnings only indicator
	if e.warningsOnly {
		indicator := lipgloss.NewStyle().Foreground(colorError).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Warnings")
		indicators = append(indicators, indicator+" "+label)
	}

	// Kind filter indicator
	if e.kindFilter != "" {
		kindStyle := lipgloss.NewStyle().Foreground(colorMuted)
		indicators = append(indicators, kindStyle.Render(e.kindFilter))
	}

	// Count info
	countStyle := lipgloss.NewStyle().Foreground(colorSubtle)
	countInfo := countStyle.Render(fmt.Sprintf("Events: %d", e.filteredCount))
	if e.filteredCount != e.totalEvents {
		countInfo = countStyle.Render(fmt.Sprintf("Events: %d/%d", e.filteredCount, e.totalEvents))
	}

	// Scroll percentage
	scrollInfo := countStyle.Render(fmt.Sprintf("%.0f%%", e.ScrollPercent()*100))

	// Build header
	header := titleStyle.Render(title)
	if len(indicators) > 0 {
		header += "  " + strings.Join(indicators, "  ")
	}
	header += "  " + countInfo + "  " + scrollInfo

	return header
}
