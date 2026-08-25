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
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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
	// drainBatchCap bounds one Update tick so a flood of incoming lines
	// never starves the UI loop; whatever doesn't fit gets the next tick.
	drainBatchCap = 1000
	// hScrollStep is how many columns ←/→ shift the viewport when wrap is
	// off. Tuned to "feels responsive" without skipping past short fields.
	hScrollStep = 10
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
	// rendered mirrors rawLines 1-to-1 with the formatted display string
	// (prefix + body) so refresh paths can join cached values instead of
	// re-running formatRawLine N times per event. Rebuilt only when the
	// display mode changes (showPodNames toggle, buffer trim).
	rendered []string
	width    int
	bodyH    int

	events chan logEvent
	cancel context.CancelFunc
	// podColors maps each streamed pod to its palette colour. Assigned by
	// position in pods (not hashed) so neighbouring replicas always land on
	// maximally distinct hues.
	podColors map[string]lipgloss.Color

	autoFollow   bool
	showPodNames bool // false → compact bar; true → "[pod-name]" prefix
	// wrap chooses between soft-wrap (true: long lines run onto the next
	// row) and truncate-with-hscroll (false: long lines are clipped, but
	// ←/→ pan horizontally). Default is off — terminal-native behaviour.
	wrap bool
	// xOffset is the leftmost visible column when wrap is off. Always 0
	// in wrap mode (clamped on toggle).
	xOffset int

	searchInput textinput.Model
	searchOpen  bool
	searchQuery string
	matches     []int // indices into rawLines
	matchIdx    int

	followKey       key.Binding
	tagsKey         key.Binding
	wrapKey         key.Binding
	hscrollLeftKey  key.Binding
	hscrollRightKey key.Binding
	searchOpenKey   key.Binding
	nextMatchKey    key.Binding
	prevMatchKey    key.Binding
	clearKey        key.Binding
	scrollKey       key.Binding
}

type logEvent struct {
	Pod  string
	Line string
	Err  error
	Done bool
}

// logEventsBatch is the result of one drain tick: every event the channel
// had ready (up to drainBatchCap) plus a flag set when the channel closed
// during the drain, so the loop knows to stop scheduling more reads.
type logEventsBatch struct {
	events []logEvent
	done   bool
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

	switch {
	case client == nil || len(pods) == 0:
		close(events)
	case len(pods) == 1:
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			streamOnePod(ctx, client, namespace, pods[0], tail, container, events)
		}()
		go func() {
			wg.Wait()
			close(events)
		}()
	default:
		// Multi-pod: merge the historical tail by timestamp before following live
		// lines, so the user sees chronologically interleaved replicas.
		go func() {
			runMergedStreams(ctx, client, namespace, pods, tail, container, events)
		}()
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
		podColors:   assignPodColors(pods),
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
		wrapKey: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "wrap"),
		),
		hscrollLeftKey: key.NewBinding(
			key.WithKeys("left"),
			key.WithHelp("←", "scroll left"),
		),
		hscrollRightKey: key.NewBinding(
			key.WithKeys("right"),
			key.WithHelp("→", "scroll right"),
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
	return waitOrDrainCmd(m.events)
}

// drainEvents pulls every event the channel currently has ready (no
// blocking past the first read), up to max. The returned done flag is
// true iff the channel was observed closed during the drain.
//
// Pulled out as a free function so it is testable without spinning up a
// tea.Program.
func drainEvents(ch <-chan logEvent, max int) ([]logEvent, bool) {
	if max <= 0 {
		return nil, false
	}
	batch := make([]logEvent, 0, 8)
	for len(batch) < max {
		select {
		case ev, ok := <-ch:
			if !ok {
				return batch, true
			}
			batch = append(batch, ev)
		default:
			return batch, false
		}
	}
	return batch, false
}

// waitOrDrainCmd blocks on the first event (so the bubbletea loop stays
// quiet while no logs flow) then drains everything else the channel has
// queued in one go. One Update tick rebuilds the viewport once instead
// of 5000 times when --tail=5000 arrives in a flood.
func waitOrDrainCmd(ch <-chan logEvent) tea.Cmd {
	return func() tea.Msg {
		first, ok := <-ch
		if !ok {
			return streamsClosedMsg{}
		}
		rest, done := drainEvents(ch, drainBatchCap-1)
		batch := make([]logEvent, 0, 1+len(rest))
		batch = append(batch, first)
		batch = append(batch, rest...)
		return logEventsBatch{events: batch, done: done}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case logEventsBatch:
		return m.onLogEventsBatch(msg)
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
	m.syncViewport()
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

// onLogEventsBatch applies every event in the batch with a single render
// at the end. This is the heart of the slow-tail fix: instead of N²
// formatRawLine calls and N viewport.SetContent invocations, we do one of
// each per drain tick.
func (m Model) onLogEventsBatch(msg logEventsBatch) (tea.Model, tea.Cmd) {
	for _, ev := range msg.events {
		switch {
		case ev.Err != nil:
			m.appendOne(ev.Pod, ev.Err.Error(), lineStreamErr)
		case ev.Done:
			m.appendOne(ev.Pod, "", lineStreamDone)
		default:
			m.appendOne(ev.Pod, ev.Line, lineLog)
		}
	}
	if m.searchQuery != "" {
		m.computeMatches()
	}
	m.syncViewport()
	if msg.done {
		// Channel was observed closed mid-drain. No further reads needed.
		return m, nil
	}
	return m, waitOrDrainCmd(m.events)
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
			m.rebuildRendered()
			m.syncViewport()
		}
		return m, nil, true
	case key.Matches(msg, m.wrapKey):
		m.toggleWrap()
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, m.hscrollLeftKey):
		m.hscroll(-1)
		m.syncViewport()
		return m, nil, true
	case key.Matches(msg, m.hscrollRightKey):
		m.hscroll(+1)
		m.syncViewport()
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
			m.syncViewport()
			return m, nil, true
		}
		m.clearLogs()
		return m, nil, true
	}
	return m, nil, false
}

// toggleWrap flips wrap mode and resets the horizontal pan because the two
// are mutually exclusive: in wrap mode there is no "off to the right" to
// scroll to. The next syncViewport sees a clean xOffset=0.
func (m *Model) toggleWrap() {
	m.wrap = !m.wrap
	if m.wrap {
		m.xOffset = 0
	}
}

// hscroll moves the visible window by direction × hScrollStep columns.
// Clamped at 0 on the left; the right is loosely bounded by the widest
// rendered line so the user can scroll the entire content into view
// without overshooting too far into empty space.
func (m *Model) hscroll(direction int) {
	if m.wrap {
		return
	}
	step := direction * hScrollStep
	m.xOffset += step
	if m.xOffset < 0 {
		m.xOffset = 0
	}
	if maxOff := m.maxXOffset(); m.xOffset > maxOff {
		m.xOffset = maxOff
	}
}

// maxXOffset is the rightmost xOffset that still keeps something on screen.
// We use the widest rendered line minus a fudge so the user always sees a
// few columns of context. Viewport width is treated as a soft floor.
func (m Model) maxXOffset() int {
	if m.wrap {
		return 0
	}
	widest := 0
	for _, line := range m.rendered {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	visible := m.viewport.Width
	if visible <= 0 {
		visible = 1
	}
	max := widest - visible/2
	if max < 0 {
		return 0
	}
	return max
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
	m.rendered = nil
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
		m.syncViewport()
		return m, nil
	}
	var cmd tea.Cmd
	m.searchInput, cmd = m.searchInput.Update(msg)
	return m, cmd
}

// appendOne adds a single event to both rawLines (the truth) and rendered
// (the display cache). Format runs once at append time; later refreshes
// just join the cache. The cap is enforced on both slices in lockstep so
// they never disagree about indices.
//
// Replaces the old appendRaw which only mutated rawLines and forced a
// full re-format on every tick.
func (m *Model) appendOne(pod, text string, kind lineKind) {
	multi := len(m.pods) > 1
	ln := logLine{pod: pod, text: text, kind: kind}
	m.rawLines = append(m.rawLines, ln)
	m.rendered = append(m.rendered, formatRawLine(ln, multi, m.showPodNames, m.podColors[ln.pod]))
	if len(m.rawLines) > maxBufferedLines {
		drop := len(m.rawLines) - maxBufferedLines
		m.rawLines = m.rawLines[drop:]
		m.rendered = m.rendered[drop:]
		// Match indices refer to the old positions; cheaper to recompute
		// on the next event than to shift them.
		m.matches = nil
		m.matchIdx = 0
	}
}

// rebuildRendered re-formats every rawLine according to the current
// display flags. Called only when a mode toggle (showPodNames) changes
// the way *every* line should look — never on the hot append path.
func (m *Model) rebuildRendered() {
	multi := len(m.pods) > 1
	if cap(m.rendered) < len(m.rawLines) {
		m.rendered = make([]string, len(m.rawLines))
	} else {
		m.rendered = m.rendered[:len(m.rawLines)]
	}
	for i, ln := range m.rawLines {
		m.rendered[i] = formatRawLine(ln, multi, m.showPodNames, m.podColors[ln.pod])
	}
}

// syncViewport pushes the cached rendered lines into the viewport,
// applying display transforms (wrap or hscroll, then optional search
// highlight). Replaces the old refreshContent and is cheap because it
// reuses the pre-formatted cache instead of re-running formatRawLine
// per line per event.
func (m *Model) syncViewport() {
	var content string
	switch {
	case m.wrap && m.viewport.Width > 0:
		wrapped := make([]string, len(m.rendered))
		for i, line := range m.rendered {
			wrapped[i] = wrapLine(line, m.viewport.Width)
		}
		content = strings.Join(wrapped, "\n")
	case m.xOffset > 0:
		shifted := make([]string, len(m.rendered))
		for i, line := range m.rendered {
			shifted[i] = applyXOffset(line, m.xOffset)
		}
		content = strings.Join(shifted, "\n")
	default:
		content = strings.Join(m.rendered, "\n")
	}
	if m.searchQuery != "" {
		content = highlightContent(content, m.searchQuery)
	}
	m.viewport.SetContent(content)
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
}

// applyXOffset drops the first n display columns of an ANSI-aware string.
// ansi.TruncateLeft preserves SGR continuity so colour codes survive the
// crop. n=0 is the identity.
func applyXOffset(s string, n int) string {
	if n <= 0 {
		return s
	}
	return ansi.TruncateLeft(s, n, "")
}

// wrapLine soft-wraps an ANSI-coloured string to width. Reuses the same
// charmbracelet/x/ansi helper as the describe view — consistent behaviour
// across the app.
func wrapLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Wrap(s, width, "")
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
func formatRawLine(ln logLine, multi, full bool, colour lipgloss.Color) string {
	switch ln.kind {
	case lineStreamErr:
		return styles.Warn.Render(fmt.Sprintf("[%s] stream error: %s", ln.pod, ln.text))
	case lineStreamDone:
		return styles.Hint.Render(fmt.Sprintf("[%s] stream closed", ln.pod))
	}

	if !multi {
		return ln.text
	}

	style := lipgloss.NewStyle().Foreground(colour)
	if full {
		return style.Bold(true).Render("["+ln.pod+"]") + " " + ln.text
	}
	return style.Render(compactPrefix) + " " + ln.text
}

// podPalette is ordered so neighbouring entries sit as far apart on the
// colour wheel as possible. Pods are coloured by position in the session's
// pod list, so the pods you actually compare side by side never share a hue
// family (the old FNV hash could hand out e.g. yellow and orange together).
var podPalette = []lipgloss.Color{
	"#7D56F4", // purple
	"#04B575", // green
	"#F2A65A", // orange
	"#56B4D4", // blue
	"#FF6B9D", // pink
	"#A3BE8C", // lime
	"#E5C07B", // yellow
	"#E06C75", // red
	"#3FC1C9", // cyan
	"#C678DD", // magenta
}

// assignPodColors deals palette colours to pods round-robin by list position.
func assignPodColors(pods []string) map[string]lipgloss.Color {
	out := make(map[string]lipgloss.Color, len(pods))
	for i, p := range pods {
		out[p] = podPalette[i%len(podPalette)]
	}
	return out
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
// of "[paused]", "[tags on]", "[wrap on]", "[→ N]" and "/query · M/N".
// Empty when nothing applies — the row stays present so the layout does
// not jiggle.
func (m Model) statusLine() string {
	bits := make([]string, 0, 4)
	if !m.autoFollow {
		bits = append(bits, styles.Warn.Render("[paused — press f to follow]"))
	}
	if m.wrap {
		bits = append(bits, styles.Hint.Render("[wrap on — press w to disable]"))
	} else if m.xOffset > 0 {
		bits = append(bits, styles.Hint.Render(fmt.Sprintf("[→ %d cols]", m.xOffset)))
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
// footer. The horizontal-scroll keys only appear when wrap is off, since
// they are a no-op in wrap mode.
func (m Model) Help() []key.Binding {
	bindings := []key.Binding{m.scrollKey, m.followKey, m.wrapKey}
	if !m.wrap {
		bindings = append(bindings, m.hscrollLeftKey, m.hscrollRightKey)
	}
	if len(m.pods) > 1 {
		bindings = append(bindings, m.tagsKey)
	}
	bindings = append(bindings,
		m.searchOpenKey, m.nextMatchKey, m.prevMatchKey, m.clearKey)
	return bindings
}

// CapturesKeys implements views.View. Returns true while the search prompt
// is focused so that 'q' / ':' reach the input as text rather than the
// global navigation.
func (m Model) CapturesKeys() bool { return m.searchOpen }

// Close cancels the streaming context, draining the goroutines, which
// eventually closes the events channel. Idempotent.
func (m Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
