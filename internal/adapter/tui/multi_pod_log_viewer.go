package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MultiPodLogEntry is a single log line tagged with its source
type MultiPodLogEntry struct {
	PodName   string
	Container string
	Line      string
}

// MultiPodLogViewer displays interleaved logs from multiple pods
type MultiPodLogViewer struct {
	pods       []string
	entries    []MultiPodLogEntry
	viewport   viewport.Model
	styles     Styles
	width      int
	height     int
	ready      bool
	following  bool
	autoScroll bool
	totalLines int
	lastSource string // tracks last pod/container for header insertion
}

// NewMultiPodLogViewer creates a new multi-pod log viewer
func NewMultiPodLogViewer(styles Styles) MultiPodLogViewer {
	return MultiPodLogViewer{
		styles:     styles,
		autoScroll: true,
		following:  true,
		entries:    make([]MultiPodLogEntry, 0),
	}
}

// SetPods initializes the viewer for a new multi-log session
func (v *MultiPodLogViewer) SetPods(pods []string) {
	v.pods = pods
	v.entries = make([]MultiPodLogEntry, 0)
	v.totalLines = 0
	v.lastSource = ""
	v.following = true
	v.autoScroll = true

	if v.ready {
		v.viewport.SetContent("Waiting for logs...")
		v.viewport.GotoTop()
	}
}

// SetSize sets the viewport size
func (v *MultiPodLogViewer) SetSize(width, height int) {
	v.width = width
	v.height = height
	v.viewport = viewport.New(width, height)
	v.viewport.Style = lipgloss.NewStyle()
	v.ready = true
	v.updateContent()
}

// AppendEntry appends a tagged log entry
func (v *MultiPodLogViewer) AppendEntry(entry MultiPodLogEntry) {
	line := strings.TrimSuffix(entry.Line, "\n")
	if line == "" {
		return
	}
	entry.Line = line

	v.entries = append(v.entries, entry)
	v.totalLines++
	v.updateContent()

	if v.following && v.autoScroll && v.ready {
		v.viewport.GotoBottom()
	}
}

func (v *MultiPodLogViewer) updateContent() {
	if !v.ready {
		return
	}

	if len(v.entries) == 0 {
		v.viewport.SetContent("Waiting for logs...")
		return
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary)

	var sb strings.Builder
	lastSource := ""

	for i, e := range v.entries {
		source := e.PodName + "/" + e.Container

		if source != lastSource {
			if lastSource != "" {
				sb.WriteString("\n")
			}
			header := fmt.Sprintf("==> %s <==", source)
			sb.WriteString(headerStyle.Render(header))
			sb.WriteString("\n")
			lastSource = source
		}

		sb.WriteString(e.Line)
		if i < len(v.entries)-1 {
			sb.WriteString("\n")
		}
	}

	v.viewport.SetContent(sb.String())
}

// Clear clears all log entries
func (v *MultiPodLogViewer) Clear() {
	v.entries = make([]MultiPodLogEntry, 0)
	v.totalLines = 0
	v.lastSource = ""
	v.pods = nil
	v.updateContent()
}

// SetFollowing sets follow mode
func (v *MultiPodLogViewer) SetFollowing(following bool) {
	v.following = following
	v.autoScroll = following
}

// IsFollowing returns whether follow mode is active
func (v *MultiPodLogViewer) IsFollowing() bool {
	return v.following
}

// ToggleFollowing toggles follow mode
func (v *MultiPodLogViewer) ToggleFollowing() bool {
	v.following = !v.following
	v.autoScroll = v.following
	if v.following && v.ready {
		v.viewport.GotoBottom()
	}
	return v.following
}

// Update handles messages
func (v MultiPodLogViewer) Update(msg tea.Msg) (MultiPodLogViewer, tea.Cmd) {
	// Disable auto-scroll and follow on manual scroll
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "up", "down", "pgup", "pgdown", "k", "j":
			v.autoScroll = false
			v.following = false
		case "G":
			v.autoScroll = true
			v.following = true
			v.viewport.GotoBottom()
			return v, nil
		case "g":
			v.autoScroll = false
			v.following = false
			v.viewport.GotoTop()
			return v, nil
		}
	}

	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

// View renders the log viewer
func (v *MultiPodLogViewer) View() string {
	if !v.ready {
		return "Loading..."
	}
	return v.viewport.View()
}

// ScrollPercent returns the scroll percentage
func (v *MultiPodLogViewer) ScrollPercent() float64 {
	return v.viewport.ScrollPercent()
}

// TotalLines returns the total number of log lines
func (v *MultiPodLogViewer) TotalLines() int {
	return v.totalLines
}

// RenderHeader returns the multi-pod log viewer header
func (v *MultiPodLogViewer) RenderHeader() string {
	title := fmt.Sprintf("Multi-Pod Logs (%d pods)", len(v.pods))

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorPrimary)

	var indicators []string

	if v.following {
		indicator := lipgloss.NewStyle().Foreground(colorSuccess).Render("◉")
		label := lipgloss.NewStyle().Foreground(colorText).Render("Following")
		indicators = append(indicators, indicator+" "+label)
	}

	infoStyle := lipgloss.NewStyle().Foreground(colorSubtle)
	linesInfo := infoStyle.Render(fmt.Sprintf("Lines: %d", v.totalLines))
	scrollInfo := infoStyle.Render(fmt.Sprintf("%.0f%%", v.ScrollPercent()*100))

	header := titleStyle.Render(title)
	if len(indicators) > 0 {
		header += "  " + strings.Join(indicators, "  ")
	}
	header += "  " + linesInfo + "  " + scrollInfo

	return header
}
