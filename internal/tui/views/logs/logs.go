// Package logs is the streaming logs view: per-pod goroutines feed lines into
// a shared channel; the view appends them to a viewport with an opt-in
// per-pod tag so multiple replicas are visually distinguishable.
//
// Beyond plain tailing it offers four quality-of-life features:
//
//   - Smart auto-follow: stays glued to the bottom while new lines arrive,
//     pauses automatically when the user scrolls up so they can read.
//   - 'f' manual toggle for the same auto-follow.
//   - '/'-search with 'n' / 'N' to jump between matches and a yellow
//     highlight on every match.
//   - 't' toggles per-pod tags. Default is a tiny coloured bar (▌) so log
//     lines stay readable; pressing 't' switches to "[pod-name]" prefixes
//     when the user actually needs to know which pod a line came from.
//     For single-pod views the prefix is omitted entirely (the name is in
//     the header) and 't' is a no-op.
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
	statusRowsClosed  = 1
	statusRowsOpen    = 2
	// compactPrefix is the tiny coloured bar shown in front of each line
	// when "tags" mode is off (default). The colour is per-pod so users can
	// still tell replicas apart without losing the line to a long name.
	compactPrefix = "▌"
)

// lineKind disambiguates the three sources we render: a real log line, a
// per-pod stream error, or a per-pod "stream closed" notice.
type lineKind int

const (
	lineLog lineKind = iota
	lineStreamErr
	lineStreamDone
)

// logLine is one entry in the in-memory backlog. We keep raw text so that
// toggling the prefix mode or running search re-formats the same source
// rather than the previously-decorated string.
type logLine struct {
	pod  string
	text string // for lineLog: the bare log line; for lineStreamErr: the error message; ignored for lineStreamDone
	kind lineKind
}

// Model is the streaming logs view.
type Model struct {
	client    *k8s.Client
	namespace string
	pods      []string
	tail      int64
	container string // "" = kubectl picks default; named container otherwise

	viewport viewport.Model
	rawLines []logLine
	width    int
	bodyH    int

	events chan logEvent
	cancel context.CancelFunc

	autoFollow   bool
	showPodNames bool // false → compact bar; true → "[pod-name]" prefix

	searchInput textinput.Model
	searchOpen  bool
	searchQuery string
	matches     []int // indices into rawLines
	matchIdx    int

	followKey     key.Binding
	tagsKey       key.Binding
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
func New(client *k8s.Client, namespace string, pods []string, tail int64, container string) Model {
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
				streamOnePod(ctx, client, namespace, p, tail, container, events)
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
		container:   container,
		viewport:    viewport.New(80, 20),
		events:      events,
		cancel:      cancel,
		autoFollow:  true,
		searchInput: si,
		followKey: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "follow"),
		),
		tagsKey: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "tag pods"),
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

func streamOnePod(ctx context.Context, client *k8s.Client, ns, pod string, tail int64, container string, ch chan<- logEvent) {
	stream, err := client.StreamPodLogs(ctx, ns, pod, tail, container)
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

func (m *Model) layoutViewport() {
	chrome := statusRowsClosed
	if m.searchOpen {
		chrome = statusRowsOpen
	}
	h := m.bodyH - chrome
	m.viewport.Width = m.width
	m.viewport.Height = max(h, minViewportHeight)
}

func (m Model) onLogEvent(msg logEvent) (tea.Model, tea.Cmd) {
	switch {
	case msg.Err != nil:
		m.appendRaw(msg.Pod, msg.Err.Error(), lineStreamErr)
	case msg.Done:
		m.appendRaw(msg.Pod, "", lineStreamDone)
	default:
		m.appendRaw(msg.Pod, msg.Line, lineLog)
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
	prevBottom := m.viewport.AtBottom()
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	nowBottom := m.viewport.AtBottom()
	if nowBottom != prevBottom {
		m.autoFollow = nowBottom
	}
	return m, cmd
}

func (m Model) handleViewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd, bool) {
	switch {
	case key.Matches(msg, m.followKey):
		m.autoFollow = !m.autoFollow
		if m.autoFollow {
			m.viewport.GotoBottom()
		}
		return m, nil, true
	case key.Matches(msg, m.tagsKey):
		// No-op for single-pod views — the prefix is suppressed there
		// regardless of this flag, so flipping it is just visual noise.
		if len(m.pods) > 1 {
			m.showPodNames = !m.showPodNames
			m.refreshContent()
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
		// Two-step clear: while a search is active (prompt open or query
		// set), 'c' cancels the search first; only when there is no search
		// does it actually wipe the buffer. Clearing the buffer must NEVER
		// touch autoFollow — the user's paused state stays paused.
		if m.searchOpen || m.searchQuery != "" {
			m.cancelSearch()
			m.refreshContent()
			return m, nil, true
		}
		m.clearLogs()
		return m, nil, true
	}
	return m, nil, false
}

func (m *Model) cancelSearch() {
	m.searchQuery = ""
	m.matches = nil
	m.matchIdx = 0
	m.searchInput.Reset()
	if m.searchOpen {
		m.searchOpen = false
		m.searchInput.Blur()
		m.layoutViewport()
	}
}

func (m *Model) clearLogs() {
	m.rawLines = nil
	m.viewport.SetContent("")
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

func (m *Model) appendRaw(pod, text string, kind lineKind) {
	m.rawLines = append(m.rawLines, logLine{pod: pod, text: text, kind: kind})
	if len(m.rawLines) > maxBufferedLines {
		m.rawLines = m.rawLines[len(m.rawLines)-maxBufferedLines:]
		// Indices in m.matches refer to old positions; if we trimmed, drop
		// them. Cheap to recompute on the next event.
		m.matches = nil
		m.matchIdx = 0
	}
}

func (m *Model) refreshContent() {
	multi := len(m.pods) > 1
	rendered := make([]string, 0, len(m.rawLines))
	for _, ln := range m.rawLines {
		rendered = append(rendered, formatRawLine(ln, multi, m.showPodNames))
	}
	content := strings.Join(rendered, "\n")
	if m.searchQuery != "" {
		content = highlightContent(content, m.searchQuery)
	}
	m.viewport.SetContent(content)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// computeMatches runs the substring search against the *raw* line text so
// matches are independent of the current prefix mode.
func (m *Model) computeMatches() {
	m.matches = nil
	if m.searchQuery == "" {
		return
	}
	for i, ln := range m.rawLines {
		if strings.Contains(ln.text, m.searchQuery) {
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

// formatRawLine turns a raw entry into the string the viewport renders.
//
// Modes:
//   - !multi: no prefix (single-pod view; pod name is in the title).
//   - multi && !full: a coloured "▌" bar — minimal but tells replicas apart.
//   - multi && full: the canonical "[pod-name] line" prefix.
//
// Stream-error and stream-closed notices keep the pod name inline so they
// remain readable regardless of the prefix mode.
func formatRawLine(ln logLine, multi, full bool) string {
	switch ln.kind {
	case lineStreamErr:
		return styles.Warn.Render(fmt.Sprintf("[%s] stream error: %s", ln.pod, ln.text))
	case lineStreamDone:
		return styles.Hint.Render(fmt.Sprintf("[%s] stream closed", ln.pod))
	}

	if !multi {
		return ln.text
	}

	colour := lipgloss.NewStyle().Foreground(podColor(ln.pod))
	if full {
		return colour.Bold(true).Render("["+ln.pod+"]") + " " + ln.text
	}
	return colour.Render(compactPrefix) + " " + ln.text
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

func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}

	body := m.viewport.View()
	if len(m.rawLines) == 0 {
		body = styles.Hint.Render(fmt.Sprintf("waiting for logs… (tail %d)", m.tail))
	}

	top := make([]string, 0, statusRowsOpen)
	if m.searchOpen {
		top = append(top, m.searchInput.View(), "")
	} else {
		top = append(top, m.statusLine())
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(top, body)...)
}

// statusLine renders the one-line status above the viewport: any combination
// of "[paused]", "[tags on]" and "/query · M/N". Empty when nothing applies
// — the row stays present so the layout does not jiggle.
func (m Model) statusLine() string {
	bits := make([]string, 0, 3)
	if !m.autoFollow {
		bits = append(bits, styles.Warn.Render("[paused — press f to follow]"))
	}
	if len(m.pods) > 1 && m.showPodNames {
		bits = append(bits, styles.Hint.Render("[tags on — press t to compact]"))
	}
	switch {
	case m.searchQuery != "" && len(m.matches) > 0:
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · %d/%d (n/N step · c cancel)", m.searchQuery, m.matchIdx+1, len(m.matches))))
	case m.searchQuery != "":
		bits = append(bits, styles.Hint.Render(fmt.Sprintf(
			"/%s · no match (c cancel)", m.searchQuery)))
	}
	return strings.Join(bits, " · ")
}

// Title implements views.View.
func (m Model) Title() string {
	base := "logs"
	if len(m.pods) == 1 {
		base += " · " + m.pods[0]
	} else {
		base += fmt.Sprintf(" · %d pods", len(m.pods))
	}
	if m.container != "" {
		base += " / " + m.container
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
	containerArg := ""
	if m.container != "" {
		containerArg = " -c " + m.container
	}
	if len(m.pods) == 1 {
		return fmt.Sprintf("kubectl logs -f%s %s -n %s --tail=%d", containerArg, m.pods[0], m.namespace, m.tail)
	}
	return fmt.Sprintf("kubectl logs -f%s --tail=%d -n %s {%s}", containerArg, m.tail, m.namespace, strings.Join(m.pods, ","))
}

// Help implements views.View. The 't' binding is only advertised when there
// is more than one pod — otherwise it is a no-op and only clutters the
// footer.
func (m Model) Help() []key.Binding {
	bindings := []key.Binding{
		m.scrollKey, m.followKey, m.searchOpenKey,
		m.nextMatchKey, m.prevMatchKey, m.clearKey,
	}
	if len(m.pods) > 1 {
		// Slot 't' right after 'f' so related toggles stay together.
		bindings = []key.Binding{
			m.scrollKey, m.followKey, m.tagsKey, m.searchOpenKey,
			m.nextMatchKey, m.prevMatchKey, m.clearKey,
		}
	}
	return bindings
}

// Close cancels the streaming context, draining the goroutines, which
// eventually closes the events channel. Idempotent.
func (m Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
