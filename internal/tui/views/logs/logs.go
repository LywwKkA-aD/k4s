// Package logs is the streaming logs view: per-pod goroutines feed lines into
// a shared channel; the view appends them to a viewport with a coloured
// per-pod prefix so multiple replicas are visually distinguishable.
//
// Beyond plain tailing it offers three quality-of-life features:
//
//   - smart auto-follow: stays glued to the bottom while new lines arrive,
//     pauses automatically when the user scrolls up so they can read.
//   - 'f' manual toggle for the same auto-follow.
//   - '/'-search with 'n' / 'N' to jump between matches and yellow
//     highlighting on every match.
package logs

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
)

const (
	minViewportHeight = 5
	maxBufferedLines  = 5000
	defaultTailLines  = int64(100)
	scannerBufferSize = 1024 * 1024
	eventBufferSize   = 256
	searchInputPrompt = "/"
	searchChromeRows  = 2 // search input + spacer
)

// Model is the streaming logs view.
type Model struct {
	client    *k8s.Client
	namespace string
	pods      []string
	tail      int64

	viewport viewport.Model
	lines    []string
	width    int
	bodyH    int

	events chan logEvent
	cancel context.CancelFunc

	autoFollow bool

	// Search state.
	searchInput textinput.Model
	searchOpen  bool
	searchQuery string
	matches     []int // indices into m.lines
	matchIdx    int

	followKey     key.Binding
	searchOpenKey key.Binding
	nextMatchKey  key.Binding
	prevMatchKey  key.Binding
	clearKey      key.Binding
	scrollKey     key.Binding
}

type logEvent struct {
	Pod  string
	Line string
	Err  error
	Done bool
}

type streamsClosedMsg struct{}

// New constructs a logs view and *immediately* starts the streaming
// goroutines. They are torn down on Close(). The shared events channel is
// closed once every pod's goroutine returns, which signals streamsClosedMsg
// to the bubbletea loop and prevents leaked Cmd reads.
//
// tail seeds each stream with the last N lines (kubectl --tail=N -f). Use 0
// or a negative value to pick up the package default.
func New(client *k8s.Client, namespace string, pods []string, tail int64) Model {
	if tail <= 0 {
		tail = defaultTailLines
	}

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan logEvent, eventBufferSize)

	if client != nil && len(pods) > 0 {
		var wg sync.WaitGroup
		for _, pod := range pods {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				streamOnePod(ctx, client, namespace, p, tail, events)
			}(pod)
		}
		go func() {
			wg.Wait()
			close(events)
		}()
	} else {
		close(events)
	}

	si := textinput.New()
	si.Prompt = searchInputPrompt
	si.CharLimit = 128
	si.Width = 40

	return Model{
		client:      client,
		namespace:   namespace,
		pods:        pods,
		tail:        tail,
		viewport:    viewport.New(80, 20),
		events:      events,
		cancel:      cancel,
		autoFollow:  true,
		searchInput: si,
		followKey: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "follow"),
		),
		searchOpenKey: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
		nextMatchKey: key.NewBinding(
			key.WithKeys("n"),
			key.WithHelp("n", "next match"),
		),
		prevMatchKey: key.NewBinding(
			key.WithKeys("N"),
			key.WithHelp("N", "prev match"),
		),
		clearKey: key.NewBinding(
			key.WithKeys("c"),
			key.WithHelp("c", "clear"),
		),
		scrollKey: key.NewBinding(
			key.WithKeys("up", "down", "pgup", "pgdn"),
			key.WithHelp("↑↓/pgup/pgdn", "scroll"),
		),
	}
}

// streamOnePod follows one pod's logs and pushes events into ch. Stops when
// ctx is cancelled or the stream ends.
func streamOnePod(ctx context.Context, client *k8s.Client, ns, pod string, tail int64, ch chan<- logEvent) {
	stream, err := client.StreamPodLogs(ctx, ns, pod, tail)
	if err != nil {
		send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
		return
	}
	defer func() { _ = stream.Close() }()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 64*1024), scannerBufferSize)

	for scanner.Scan() {
		if !send(ctx, ch, logEvent{Pod: pod, Line: scanner.Text()}) {
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
		return
	}
	send(ctx, ch, logEvent{Pod: pod, Done: true})
}

func send(ctx context.Context, ch chan<- logEvent, ev logEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case ch <- ev:
		return true
	}
}

// Init returns the first pump cmd; goroutines were started in New.
func (m Model) Init() tea.Cmd {
	return waitForEventCmd(m.events)
}

func waitForEventCmd(ch <-chan logEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return streamsClosedMsg{}
		}
		return ev
	}
}

// Update is the message router; per-message work lives in helpers so each
// stays small (gocyclo-friendly) and unit-testable.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case logEvent:
		return m.onLogEvent(msg)
	case streamsClosedMsg:
		return m, nil
	case tea.KeyMsg:
		return m.onKey(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) onWindowSize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width = msg.Width
	m.bodyH = msg.Height
	m.layoutViewport()
	m.refreshContent()
	return m, nil
}

// layoutViewport re-sizes the inner viewport, leaving room for the search
// input / status banner when search is open.
func (m *Model) layoutViewport() {
	h := m.bodyH
	if m.searchOpen {
		h -= searchChromeRows
	}
	m.viewport.Width = m.width
	m.viewport.Height = max(h, minViewportHeight)
}

func (m Model) onLogEvent(msg logEvent) (tea.Model, tea.Cmd) {
	switch {
	case msg.Err != nil:
		m.appendLine(styles.Warn.Render(fmt.Sprintf("[%s] stream error: %v", msg.Pod, msg.Err)))
	case msg.Done:
		m.appendLine(styles.Hint.Render(fmt.Sprintf("[%s] stream closed", msg.Pod)))
	default:
		m.appendLine(formatLine(msg.Pod, msg.Line, len(m.pods) > 1))
	}
	if m.searchQuery != "" {
		m.computeMatches()
	}
	m.refreshContent()
	return m, waitForEventCmd(m.events)
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.searchOpen {
		return m.onSearchKey(msg)
	}
	if next, cmd, handled := m.handleViewKey(msg); handled {
		return next, cmd
	}
	// Default: forward to viewport, then sync auto-follow with the cursor's
	// new position. We only do this on key messages so log-event-driven
	// GotoBottom() does not flip autoFollow.
	prevBottom := m.viewport.AtBottom()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	nowBottom := m.viewport.AtBottom()
	if nowBottom != prevBottom {
		m.autoFollow = nowBottom
	}
	return m, cmd
}

// handleViewKey deals with the view's own bindings; returns handled=true if
// the key matched one of them so onKey skips the viewport-forwarding fallthrough.
func (m Model) handleViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.followKey):
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.viewport.GotoBottom()
		}
		return m, nil, true
	case key.Matches(msg, m.searchOpenKey):
		m.searchOpen = true
		m.searchInput.Reset()
		m.searchInput.SetValue(m.searchQuery)
		m.searchInput.Focus()
		m.layoutViewport()
		return m, textinput.Blink, true
	case key.Matches(msg, m.nextMatchKey):
		m.gotoMatch(+1)
		return m, nil, true
	case key.Matches(msg, m.prevMatchKey):
		m.gotoMatch(-1)
		return m, nil, true
	case key.Matches(msg, m.clearKey):
		m.lines = nil
		m.matches = nil
		m.matchIdx = 0
		m.searchQuery = ""
		m.searchInput.Reset()
		m.viewport.SetContent("")
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) onSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
		return m, nil
	case tea.KeyEnter:
		m.searchQuery = strings.TrimSpace(m.searchInput.Value())
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
		m.computeMatches()
		m.matchIdx = 0
		if len(m.matches) > 0 {
			m.scrollToLine(m.matches[0])
			m.autoFollow = false
		}
		m.refreshContent()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

func (m *Model) appendLine(line string) {
	m.lines = append(m.lines, line)
	if len(m.lines) > maxBufferedLines {
		m.lines = m.lines[len(m.lines)-maxBufferedLines:]
		// Indices in m.matches refer to old positions; if we trimmed, drop them.
		// Cheap to recompute on the next event.
		m.matches = nil
		m.matchIdx = 0
	}
}

func (m *Model) refreshContent() {
	content := strings.Join(m.lines, "\n")
	if m.searchQuery != "" {
		content = highlightContent(content, m.searchQuery)
	}
	m.viewport.SetContent(content)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

func (m *Model) computeMatches() {
	m.matches = nil
	if m.searchQuery == "" {
		return
	}
	for i, line := range m.lines {
		if strings.Contains(line, m.searchQuery) {
			m.matches = append(m.matches, i)
		}
	}
	if m.matchIdx >= len(m.matches) {
		m.matchIdx = 0
	}
}

func (m *Model) gotoMatch(direction int) {
	if len(m.matches) == 0 {
		return
	}
	m.matchIdx = (m.matchIdx + direction + len(m.matches)) % len(m.matches)
	m.scrollToLine(m.matches[m.matchIdx])
	m.autoFollow = false
}

func (m *Model) scrollToLine(line int) {
	target := line - m.viewport.Height/2
	if target < 0 {
		target = 0
	}
	m.viewport.SetYOffset(target)
}

// formatLine prepends a stable, coloured "[pod-name]" tag when there is more
// than one pod; for a single pod the name is already in the header.
func formatLine(pod, line string, showPrefix bool) string {
	if !showPrefix {
		return line
	}
	prefix := lipgloss.NewStyle().
		Foreground(podColor(pod)).
		Bold(true).
		Render("[" + pod + "]")
	return prefix + " " + line
}

// podColor maps a pod name to a colour from a curated palette via FNV-1a so
// the same pod always gets the same colour across runs.
func podColor(name string) lipgloss.Color {
	palette := []lipgloss.Color{
		"#7D56F4", "#04B575", "#F2A65A", "#FF6B9D", "#56B4D4",
		"#E5C07B", "#98C379", "#C678DD", "#56B6C2", "#E06C75",
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return palette[int(h.Sum32())%len(palette)]
}

func highlightContent(content, query string) string {
	if query == "" {
		return content
	}
	return strings.ReplaceAll(content, query, styles.Highlight.Render(query))
}

// View renders the search input + status banner + viewport.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}

	body := m.viewport.View()
	if len(m.lines) == 0 {
		body = styles.Hint.Render(fmt.Sprintf("waiting for logs… (tail %d)", m.tail))
	}

	parts := []string{}
	if m.searchOpen {
		parts = append(parts, m.searchInput.View(), "")
	}
	if !m.autoFollow {
		parts = append(parts, styles.Warn.Render("[paused — press f to resume]"))
	}
	parts = append(parts, body)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// Title implements views.View.
func (m Model) Title() string {
	base := "logs"
	if len(m.pods) == 1 {
		base += " · " + m.pods[0]
	} else {
		base += fmt.Sprintf(" · %d pods", len(m.pods))
	}
	if m.searchQuery != "" {
		if len(m.matches) > 0 {
			base += fmt.Sprintf(" · /%s · %d/%d", m.searchQuery, m.matchIdx+1, len(m.matches))
		} else {
			base += " · /" + m.searchQuery + " · no match"
		}
	}
	if !m.autoFollow {
		base += " · paused"
	}
	return base
}

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if len(m.pods) == 1 {
		return fmt.Sprintf("kubectl logs -f %s -n %s --tail=%d", m.pods[0], m.namespace, m.tail)
	}
	return fmt.Sprintf("kubectl logs -f --tail=%d -n %s {%s}", m.tail, m.namespace, strings.Join(m.pods, ","))
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{
		m.scrollKey, m.followKey, m.searchOpenKey,
		m.nextMatchKey, m.prevMatchKey, m.clearKey,
	}
}

// Close cancels the streaming context, draining the goroutines, which
// eventually closes the events channel. Idempotent.
func (m Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
