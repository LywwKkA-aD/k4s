package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ansiRegex matches ANSI escape sequences
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// LogViewer is the model for viewing pod logs
type LogViewer struct {
	podName       string
	namespace     string
	container     string
	containers    []string
	logs          []string
	viewport      viewport.Model
	styles        Styles
	width         int
	height        int
	ready         bool
	following     bool
	timestamps    bool
	searchQuery   string
	searching     bool
	tailLines     int64
	autoScroll    bool
	totalLines    int
	filteredLines []int // Indices of lines matching search
}

// NewLogViewer creates a new log viewer
func NewLogViewer(styles Styles) LogViewer {
	return LogViewer{
		styles:     styles,
		tailLines:  500,
		autoScroll: true,
		following:  false, // Default: no follow
		logs:       make([]string, 0),
	}
}

// SetPod sets the pod to view logs for
func (l *LogViewer) SetPod(podName, namespace string, containers []string) {
	l.podName = podName
	l.namespace = namespace
	l.containers = containers
	l.logs = make([]string, 0)
	l.totalLines = 0
	l.filteredLines = nil
	l.searchQuery = ""
	l.searching = false
	l.following = false  // Reset follow mode when changing pods
	l.autoScroll = true  // Start at bottom

	// Default to first container
	if len(containers) > 0 {
		l.container = containers[0]
	} else {
		l.container = ""
	}

	if l.ready {
		l.viewport.SetContent("Loading logs...")
		l.viewport.GotoTop()
	}
}

// SetContainer sets the current container
func (l *LogViewer) SetContainer(container string) {
	l.container = container
	l.logs = make([]string, 0)
	l.totalLines = 0
	if l.ready {
		l.viewport.SetContent("Loading logs...")
		l.viewport.GotoTop()
	}
}

// Container returns the current container name
func (l *LogViewer) Container() string {
	return l.container
}

// Containers returns all container names
func (l *LogViewer) Containers() []string {
	return l.containers
}

// PodName returns the current pod name
func (l *LogViewer) PodName() string {
	return l.podName
}

// SetSize sets the viewport size
func (l *LogViewer) SetSize(width, height int) {
	l.width = width
	l.height = height
	l.viewport = viewport.New(width, height)
	l.viewport.Style = lipgloss.NewStyle()
	l.ready = true
	l.updateContent()
}

// SetLogs sets the initial logs content
func (l *LogViewer) SetLogs(content string) {
	l.logs = make([]string, 0)
	if content != "" {
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if line != "" {
				l.logs = append(l.logs, line)
			}
		}
	}
	l.totalLines = len(l.logs)
	l.updateContent()

	// Auto-scroll to bottom
	if l.autoScroll && l.ready {
		l.viewport.GotoBottom()
	}
}

// AppendLog appends a log line (used for streaming)
func (l *LogViewer) AppendLog(line string) {
	// Remove trailing newline if present
	line = strings.TrimSuffix(line, "\n")
	if line == "" {
		return
	}

	l.logs = append(l.logs, line)
	l.totalLines = len(l.logs)
	l.updateContent()

	// Auto-scroll to bottom if following
	if l.following && l.autoScroll && l.ready {
		l.viewport.GotoBottom()
	}
}

// SetFollowing sets the follow mode
func (l *LogViewer) SetFollowing(following bool) {
	l.following = following
	l.autoScroll = following
}

// IsFollowing returns whether follow mode is active
func (l *LogViewer) IsFollowing() bool {
	return l.following
}

// ToggleFollowing toggles follow mode
func (l *LogViewer) ToggleFollowing() bool {
	l.following = !l.following
	l.autoScroll = l.following
	if l.following && l.ready {
		l.viewport.GotoBottom()
	}
	return l.following
}

// SetTimestamps sets whether to show timestamps
func (l *LogViewer) SetTimestamps(timestamps bool) {
	l.timestamps = timestamps
}

// ToggleTimestamps toggles timestamp display
func (l *LogViewer) ToggleTimestamps() bool {
	l.timestamps = !l.timestamps
	return l.timestamps
}

// Timestamps returns whether timestamps are enabled
func (l *LogViewer) Timestamps() bool {
	return l.timestamps
}

// TailLines returns the number of tail lines (fixed at 500)
func (l *LogViewer) TailLines() int64 {
	return l.tailLines
}

// SetSearchQuery sets the search query
func (l *LogViewer) SetSearchQuery(query string) {
	l.searchQuery = query
	l.updateFilteredLines()
	l.updateContent()
}

// IsSearching returns whether search mode is active
func (l *LogViewer) IsSearching() bool {
	return l.searching
}

// SetSearching sets search mode
func (l *LogViewer) SetSearching(searching bool) {
	l.searching = searching
	if !searching {
		l.searchQuery = ""
		l.filteredLines = nil
		l.updateContent()
	}
}

// SearchQuery returns the current search query
func (l *LogViewer) SearchQuery() string {
	return l.searchQuery
}

// Clear clears the logs
func (l *LogViewer) Clear() {
	l.logs = make([]string, 0)
	l.totalLines = 0
	l.filteredLines = nil
	l.updateContent()
}

func (l *LogViewer) updateFilteredLines() {
	if l.searchQuery == "" {
		l.filteredLines = nil
		return
	}

	query := strings.ToLower(l.searchQuery)
	l.filteredLines = make([]int, 0)

	for i, line := range l.logs {
		if strings.Contains(strings.ToLower(line), query) {
			l.filteredLines = append(l.filteredLines, i)
		}
	}
}

func (l *LogViewer) updateContent() {
	if !l.ready {
		return
	}

	if len(l.logs) == 0 {
		l.viewport.SetContent("No logs available")
		return
	}

	var sb strings.Builder
	query := strings.ToLower(l.searchQuery)

	for i, line := range l.logs {
		// Wrap long lines using visible width (ignoring ANSI codes)
		if l.width > 0 && visibleWidth(line) > l.width {
			wrappedLines := l.wrapLine(line, l.width)
			for j, wrappedLine := range strings.Split(wrappedLines, "\n") {
				// Apply search highlighting if searching
				if query != "" && strings.Contains(strings.ToLower(wrappedLine), query) {
					wrappedLine = l.highlightSearch(wrappedLine, query)
				}
				if j > 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(wrappedLine)
			}
		} else {
			// Apply search highlighting if searching
			if query != "" && strings.Contains(strings.ToLower(line), query) {
				line = l.highlightSearch(line, query)
			}
			sb.WriteString(line)
		}
		if i < len(l.logs)-1 {
			sb.WriteString("\n")
		}
	}

	l.viewport.SetContent(sb.String())
}

// visibleWidth returns the visible width of a string, ignoring ANSI escape codes
func visibleWidth(s string) int {
	return len(ansiRegex.ReplaceAllString(s, ""))
}

// wrapLine wraps a line to fit within the specified width, preserving ANSI codes
func (l *LogViewer) wrapLine(line string, width int) string {
	if visibleWidth(line) <= width {
		return line
	}

	var result strings.Builder
	var currentLine strings.Builder
	currentWidth := 0
	i := 0

	for i < len(line) {
		// Check if we're at an ANSI escape sequence
		if line[i] == '\x1b' {
			// Find the end of the ANSI sequence
			match := ansiRegex.FindStringIndex(line[i:])
			if match != nil && match[0] == 0 {
				// Write the ANSI sequence without counting its width
				seq := line[i : i+match[1]]
				currentLine.WriteString(seq)
				i += match[1]
				continue
			}
		}

		// Regular character
		if currentWidth >= width {
			// Start a new line
			result.WriteString(currentLine.String())
			result.WriteString("\n")
			currentLine.Reset()
			currentWidth = 0
		}

		currentLine.WriteByte(line[i])
		currentWidth++
		i++
	}

	// Write remaining content
	if currentLine.Len() > 0 {
		result.WriteString(currentLine.String())
	}

	return result.String()
}

func (l *LogViewer) highlightSearch(line, query string) string {
	// Simple case-insensitive highlighting
	lowerLine := strings.ToLower(line)
	idx := strings.Index(lowerLine, query)
	if idx == -1 {
		return line
	}

	highlightStyle := lipgloss.NewStyle().
		Background(colorWarning).
		Foreground(lipgloss.Color("#000000"))

	// Build highlighted line
	result := line[:idx]
	result += highlightStyle.Render(line[idx : idx+len(query)])
	result += line[idx+len(query):]

	return result
}

// Update handles messages
func (l *LogViewer) Update(msg tea.Msg) (LogViewer, tea.Cmd) {
	// Disable auto-scroll and follow on manual scroll
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "down", "pgup", "pgdown", "k", "j":
			l.autoScroll = false
			l.following = false
		case "G":
			// Go to bottom re-enables auto-scroll and follow
			l.autoScroll = true
			l.following = true
			l.viewport.GotoBottom()
			return *l, nil
		case "g":
			// Go to top disables auto-scroll and follow
			l.autoScroll = false
			l.following = false
			l.viewport.GotoTop()
			return *l, nil
		}
	}

	var cmd tea.Cmd
	l.viewport, cmd = l.viewport.Update(msg)
	return *l, cmd
}

// View renders the log viewer
func (l *LogViewer) View() string {
	if !l.ready {
		return "Loading..."
	}
	return l.viewport.View()
}

// ScrollPercent returns the scroll percentage
func (l *LogViewer) ScrollPercent() float64 {
	return l.viewport.ScrollPercent()
}

// TotalLines returns the total number of log lines
func (l *LogViewer) TotalLines() int {
	return l.totalLines
}

// MatchCount returns the number of search matches
func (l *LogViewer) MatchCount() int {
	return len(l.filteredLines)
}

// RenderHeader returns the log viewer header
func (l *LogViewer) RenderHeader() string {
	// Pod and container info
	title := fmt.Sprintf("Logs: %s", l.podName)
	if l.container != "" {
		title += fmt.Sprintf(" / %s", l.container)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary)

	// Status indicators
	var indicators []string

	// Follow indicator
	if l.following {
		indicator := lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Following")
		indicators = append(indicators, indicator+" "+label)
	}

	// Timestamps indicator
	if l.timestamps {
		indicator := lipgloss.NewStyle().Foreground(colorPrimary).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Timestamps")
		indicators = append(indicators, indicator+" "+label)
	}

	// Lines count
	infoStyle := lipgloss.NewStyle().Foreground(colorSubtle)
	linesInfo := infoStyle.Render(fmt.Sprintf("Lines: %d", l.totalLines))

	// Scroll percentage
	scrollInfo := infoStyle.Render(fmt.Sprintf("%.0f%%", l.ScrollPercent()*100))

	// Search info
	searchInfo := ""
	if l.searchQuery != "" {
		searchStyle := lipgloss.NewStyle().Foreground(colorMuted)
		searchInfo = "  " + searchStyle.Render(fmt.Sprintf("Search: '%s' (%d)", l.searchQuery, l.MatchCount()))
	}

	// Build header
	header := titleStyle.Render(title)
	if len(indicators) > 0 {
		header += "  " + strings.Join(indicators, "  ")
	}
	header += "  " + linesInfo + searchInfo + "  " + scrollInfo

	return header
}
