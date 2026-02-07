package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CrictlLogViewer displays container logs from crictl
type CrictlLogViewer struct {
	viewport      viewport.Model
	styles        Styles
	containerID   string
	containerName string
	nodeName      string
	logs          string
	logLines      []string
	tailLines     int64
	timestamps    bool
	following     bool
	width         int
	height        int
	ready         bool
	searchQuery   string
	matchCount    int
}

// NewCrictlLogViewer creates a new crictl log viewer
func NewCrictlLogViewer(styles Styles) CrictlLogViewer {
	return CrictlLogViewer{
		styles:    styles,
		tailLines: 500,
	}
}

// SetContainer sets the container to view logs for
func (l *CrictlLogViewer) SetContainer(containerID, containerName, nodeName string) {
	l.containerID = containerID
	l.containerName = containerName
	l.nodeName = nodeName
	l.logs = ""
	l.logLines = nil
	l.following = false
	l.searchQuery = ""
	l.matchCount = 0
	if l.ready {
		l.viewport.SetContent("")
		l.viewport.GotoBottom()
	}
}

// SetSize sets the viewport size
func (l *CrictlLogViewer) SetSize(width, height int) {
	l.width = width
	l.height = height
	if !l.ready {
		l.viewport = viewport.New(width, height-2)
		l.viewport.Style = lipgloss.NewStyle()
		l.ready = true
	} else {
		l.viewport.Width = width
		l.viewport.Height = height - 2
	}
}

// SetLogs sets the log content
func (l *CrictlLogViewer) SetLogs(logs string) {
	l.logs = logs
	l.logLines = strings.Split(logs, "\n")
	l.updateContent()
	if l.ready {
		l.viewport.GotoBottom()
	}
}

// AppendLog appends a line to the logs (for streaming)
func (l *CrictlLogViewer) AppendLog(line string) {
	if l.logs == "" {
		l.logs = line
	} else {
		l.logs = l.logs + "\n" + line
	}
	l.logLines = append(l.logLines, line)
	l.updateContent()
	if l.ready && l.following {
		l.viewport.GotoBottom()
	}
}

// Clear clears the log viewer
func (l *CrictlLogViewer) Clear() {
	l.containerID = ""
	l.containerName = ""
	l.nodeName = ""
	l.logs = ""
	l.logLines = nil
	l.following = false
	l.searchQuery = ""
	l.matchCount = 0
	if l.ready {
		l.viewport.SetContent("")
	}
}

// SetSearchQuery sets the search query and updates highlighting
func (l *CrictlLogViewer) SetSearchQuery(query string) {
	l.searchQuery = query
	l.updateContent()
}

// MatchCount returns the number of search matches
func (l *CrictlLogViewer) MatchCount() int {
	return l.matchCount
}

// updateContent updates the viewport content with search highlighting
func (l *CrictlLogViewer) updateContent() {
	if !l.ready {
		return
	}

	if len(l.logLines) == 0 {
		l.viewport.SetContent("No logs available")
		return
	}

	var sb strings.Builder
	query := strings.ToLower(l.searchQuery)
	l.matchCount = 0

	for i, line := range l.logLines {
		displayLine := line
		if query != "" && strings.Contains(strings.ToLower(line), query) {
			l.matchCount++
			displayLine = l.highlightSearch(line, query)
		}
		sb.WriteString(displayLine)
		if i < len(l.logLines)-1 {
			sb.WriteString("\n")
		}
	}

	l.viewport.SetContent(sb.String())
}

// highlightSearch highlights search matches in a line
func (l *CrictlLogViewer) highlightSearch(line, query string) string {
	lowerLine := strings.ToLower(line)
	idx := strings.Index(lowerLine, query)
	if idx == -1 {
		return line
	}

	highlightStyle := lipgloss.NewStyle().
		Background(colorWarning).
		Foreground(lipgloss.Color("#000000"))

	result := line[:idx]
	result += highlightStyle.Render(line[idx : idx+len(query)])
	result += line[idx+len(query):]

	return result
}

// ContainerID returns the current container ID
func (l *CrictlLogViewer) ContainerID() string {
	return l.containerID
}

// ContainerName returns the current container name
func (l *CrictlLogViewer) ContainerName() string {
	return l.containerName
}

// TailLines returns the tail lines setting
func (l *CrictlLogViewer) TailLines() int64 {
	return l.tailLines
}

// Timestamps returns whether timestamps are enabled
func (l *CrictlLogViewer) Timestamps() bool {
	return l.timestamps
}

// ToggleTimestamps toggles timestamp display
func (l *CrictlLogViewer) ToggleTimestamps() {
	l.timestamps = !l.timestamps
}

// IsFollowing returns whether follow mode is active
func (l *CrictlLogViewer) IsFollowing() bool {
	return l.following
}

// ToggleFollowing toggles follow mode
func (l *CrictlLogViewer) ToggleFollowing() bool {
	l.following = !l.following
	return l.following
}

// Update handles messages
func (l CrictlLogViewer) Update(msg tea.Msg) (CrictlLogViewer, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "down", "pgup", "pgdown", "k", "j":
			l.following = false
		case "g":
			l.following = false
			l.viewport.GotoTop()
			return l, nil
		case "G":
			l.following = true
			l.viewport.GotoBottom()
			return l, nil
		}
	}

	l.viewport, cmd = l.viewport.Update(msg)
	return l, cmd
}

// View renders the log viewer
func (l *CrictlLogViewer) View() string {
	if !l.ready {
		return "Initializing..."
	}
	return l.viewport.View()
}

// RenderHeader renders the log viewer header
func (l *CrictlLogViewer) RenderHeader() string {
	var parts []string

	// Container info
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)
	parts = append(parts, nameStyle.Render(fmt.Sprintf("Container: %s", l.containerName)))

	// Node
	if l.nodeName != "" {
		nodeStyle := lipgloss.NewStyle().Foreground(colorSubtle)
		parts = append(parts, nodeStyle.Render(fmt.Sprintf("Node: %s", l.nodeName)))
	}

	// Follow indicator
	if l.following {
		indicator := lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Following")
		parts = append(parts, indicator+" "+label)
	}

	// Timestamps indicator
	if l.timestamps {
		indicator := lipgloss.NewStyle().Foreground(colorPrimary).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Timestamps")
		parts = append(parts, indicator+" "+label)
	}

	// Search info
	if l.searchQuery != "" {
		searchStyle := lipgloss.NewStyle().Foreground(colorMuted)
		parts = append(parts, searchStyle.Render(fmt.Sprintf("Search: '%s' (%d)", l.searchQuery, l.matchCount)))
	}

	return strings.Join(parts, "  ")
}
