// Package logs is the streaming logs view: per-pod goroutines feed lines
// into a shared channel; the view appends them to a viewport with an
// opt-in per-pod tag so multiple replicas are visually distinguishable.
//
// Every line is made inert on the way in (see internal/sanitize): what a
// workload writes to its log never reaches the terminal as an escape.
// Beyond plain tailing the view offers:
//
//   - Smart auto-follow: stays glued to the bottom while new lines arrive,
//     pauses when the user scrolls up, and a paused view keeps its first
//     visible line while old lines scroll out of the buffer.
//   - 'f' manual toggle for the same auto-follow.
//   - '/'-search, smart-case, with 'n' / 'N' stepping through every
//     occurrence, wrapped lines included.
//   - 't' toggles per-pod tags: a coloured bar (▌) by default, "[pod-name]"
//     when on. Single-pod views have no prefix and 't' is a no-op.
//   - 'p' switches a single pod's stream to the previous instance of its
//     container (kubectl logs --previous) and back.
package logs

import (
	"fmt"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/async"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	minViewportHeight = 5
	defaultTailLines  = int64(100)
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

// logLine is one entry in the in-memory backlog. We keep the sanitized
// text so that toggling the prefix mode or running search re-formats the
// same source rather than the previously-decorated string.
type logLine struct {
	pod  string
	text string // lineLog: the line; lineStreamErr: the error; lineStreamDone: ""
	kind lineKind
}

// Model is the streaming logs view.
type Model struct {
	// src opens the streams; nil without a kubeconfig.
	src       streamer
	crash     crashLog
	namespace string
	pods      []string
	tail      int64
	container string // "" = kubectl picks default; named container otherwise
	// target is the deployment or service the pods were resolved from,
	// zero for a single pod; Command shows its selector.
	target k8s.LogTarget

	sess session
	// mint names this view instance; each session and opening takes its
	// Next, so a reply from another logs view, or from a stream this one
	// replaced, is never taken.
	mint async.Token
	// previous is set while the stream shows the previous instance of the
	// container; opening is the request for it in flight, if any.
	previous bool
	opening  opening

	viewport viewport.Model
	rawLines []logLine
	// rendered mirrors rawLines 1-to-1 with the formatted display string
	// (prefix + body), so a refresh joins cached values instead of
	// formatting every line again. Lines with a search match are painted
	// afresh at sync time.
	rendered []string
	// rowStart[i] is the viewport row line i starts on, and its last entry
	// the row count: wrapped lines take several rows.
	rowStart []int
	width    int
	bodyH    int

	// podColors maps each streamed pod to its palette colour. Assigned by
	// position in pods (not hashed) so neighbouring replicas always land on
	// maximally distinct hues.
	podColors map[string]styles.PodColorID

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
	search      query
	matches     []match // every occurrence, in backlog order
	matchIdx    int

	keys keyMap
}

type logEvent struct {
	Pod  string
	Line string
	Err  error
	Done bool
}

// logEventsBatch is the result of one drain tick: every event the channel
// had ready (up to drainBatchCap) plus a flag set when the channel closed
// during the drain, so the loop knows to stop scheduling more reads. tok
// names the session it was read from.
type logEventsBatch struct {
	tok    async.Token
	events []logEvent
	done   bool
}

// streamsClosedMsg says the session tok has no streams left.
type streamsClosedMsg struct{ tok async.Token }

// Option changes how New builds the view.
type Option func(*Model)

// WithTarget names the deployment or service the pods were resolved from,
// so the footer shows the selector command the stream stands for.
func WithTarget(t k8s.LogTarget) Option {
	return func(m *Model) { m.target = t }
}

// New constructs a logs view and *immediately* starts the streaming
// goroutines. They are torn down on Close(). The shared events channel is
// closed once every pod's goroutine returns, which signals streamsClosedMsg
// to the bubbletea loop and prevents leaked Cmd reads.
//
// The tail is per pod, and clamped so that all pods together start with no
// more than the buffer holds (views.ClampTail).
func New(client *k8s.Client, namespace string, pods []string, tail int64, container string, opts ...Option) Model {
	if tail <= 0 {
		tail = defaultTailLines
	}

	si := textinput.New()
	si.Prompt = searchInputPrompt
	si.CharLimit = 128
	si.SetWidth(40)
	si.SetStyles(styles.TextInput())

	m := Model{
		crash:       defaultCrashLog(),
		namespace:   namespace,
		pods:        pods,
		tail:        views.ClampTail(tail, len(pods)),
		container:   container,
		viewport:    newViewport(),
		podColors:   assignPodColors(pods),
		autoFollow:  true,
		searchInput: si,
		keys:        newKeyMap(),
	}
	if client != nil {
		m.src = client
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.mint = async.New().Next()
	m.sess = startSession(m.mint, m.spec())
	return m
}

// withSource streams from src instead of the client, for tests.
func withSource(src streamer) Option {
	return func(m *Model) { m.src = src }
}

// withCrashLog sends recovered panics to c instead of the k4s log, for
// tests.
func withCrashLog(c crashLog) Option {
	return func(m *Model) { m.crash = c }
}

// newViewport returns the log viewport. Panning belongs to the view's own
// ←/→ hscroll, so the viewport's h/l panning stays off.
func newViewport() viewport.Model {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SetHorizontalStep(0)
	return vp
}

// Init starts reading the first session's events.
func (m Model) Init() tea.Cmd {
	return waitOrDrainCmd(m.sess)
}

// Update implements views.View.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case logEventsBatch:
		return m.onLogEventsBatch(msg)
	case streamsClosedMsg:
		return m, nil
	case previousOpenedMsg:
		return m.onPreviousOpened(msg)
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.PasteMsg:
		return m.onPaste(msg)
	}
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m Model) onWindowSize(msg tea.WindowSizeMsg) (views.View, tea.Cmd) {
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
	m.viewport.SetWidth(m.width)
	m.viewport.SetHeight(max(h, minViewportHeight))
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
	if m.previous {
		base += " · previous"
	}
	if m.search.text != "" {
		if len(m.matches) > 0 {
			base += fmt.Sprintf(" · /%s · %d/%d", m.search.text, m.matchIdx+1, len(m.matches))
		} else {
			base += " · /" + m.search.text + " · no match"
		}
	}
	if !m.autoFollow {
		base += " · paused"
	}
	return base
}

// Command implements views.View: the stream's kubectl logs command, with
// --previous instead of -f while the previous instance shows.
func (m Model) Command() kubectl.Cmd {
	c := views.LogsCommand(m.namespace, m.pods, m.container, m.tail, m.target)
	if m.previous {
		return previousCommand(c)
	}
	return c
}

// Help implements views.View. 't' is only advertised when there is more
// than one pod and 'p' only for one, where each does something. The
// horizontal-scroll keys only appear when wrap is off, since they are a
// no-op in wrap mode.
func (m Model) Help() []key.Binding {
	k := m.keys
	bindings := []key.Binding{k.scroll, k.follow, k.wrap}
	if !m.wrap {
		bindings = append(bindings, k.left, k.right)
	}
	if len(m.pods) > 1 {
		bindings = append(bindings, k.tags)
	}
	bindings = append(bindings, k.searchOpen, k.next, k.prev, k.clear)
	if len(m.pods) == 1 {
		bindings = append(bindings, m.previousKey())
	}
	return bindings
}

// CapturesKeys implements views.View. Returns true while the search prompt
// is focused so that 'q' / ':' reach the input as text rather than the
// global navigation.
func (m Model) CapturesKeys() bool { return m.searchOpen }

// Close cancels the streams, and a previous-instance request in flight,
// draining the goroutines, which eventually closes the events channel.
// Idempotent.
func (m Model) Close() error {
	m.sess.stop()
	m.opening.stop()
	return nil
}
