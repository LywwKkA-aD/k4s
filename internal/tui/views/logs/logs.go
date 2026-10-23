// Package logs is the streaming logs view: per-pod goroutines feed lines
// into a shared channel; the view appends them to a viewport with an
// opt-in per-pod tag so multiple replicas are visually distinguishable.
//
// Every line is made inert on the way in (see internal/sanitize): what a
// workload writes to its log never reaches the terminal as an escape.
// Beyond plain tailing the view offers:
//
//   - Logs that keep up: a deployment's or a service's pods are resolved
//     again every 10s, so new pods join (kubectl logs -l does not follow
//     them), and every stream that ends says why: the pod was deleted,
//     completed or failed, its container restarted (reattached), or the
//     stream dropped (reconnected without repeating a line).
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
	"time"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
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

// lineKind tells apart what the backlog holds and what the streams send:
// a workload's line, an error, a notice of k4s's own about a stream
// (muted, or a warning for one that failed), and the end of a stream,
// which never reaches the backlog as such.
type lineKind int

const (
	lineLog lineKind = iota
	lineStreamErr
	lineNotice
	lineAlert
	lineStreamDone
)

// logLine is one entry in the in-memory backlog. We keep the sanitized
// text so that toggling the prefix mode or running search re-formats the
// same source rather than the previously-decorated string.
type logLine struct {
	pod  string
	text string // the line, the error or the notice
	kind lineKind
}

// Model is the streaming logs view.
type Model struct {
	// src opens the streams and res resolves pods again and reads their
	// state; both nil without a kubeconfig.
	src       streamer
	res       resolver
	clock     async.Clock
	crash     crashLog
	namespace string
	// pods are the pods streamed or waiting to, by name.
	pods      []string
	tail      int64
	container string // "" = kubectl picks default; named container otherwise
	// target is the deployment or service the pods were resolved from,
	// zero for a single pod; Command shows its selector.
	target k8s.LogTarget
	// multi is set once more than one pod streams: lines carry a prefix.
	multi bool

	sess session
	// mint names this view instance; each session and opening takes its
	// Next, so a reply from another logs view, or from a stream this one
	// replaced, is never taken.
	mint    async.Token
	streams tracker
	// follow re-resolves a deployment's or a service's pods every 10s;
	// explain is the check that tells why streams ended.
	follow  async.Poller
	explain explainer
	join    joinState
	// startup is what Init sends besides the first read.
	startup tea.Cmd
	// previous is set while the stream shows the previous instance of the
	// container; opening is the request for it in flight, if any;
	// askPrevious asks for it at the start (WithPrevious).
	previous    bool
	opening     opening
	askPrevious bool

	viewport viewport.Model
	rawLines []logLine
	// rendered mirrors rawLines 1-to-1 with the formatted display string
	// (prefix + body), so a refresh joins cached values instead of
	// formatting every line again. Lines with a search match are painted
	// afresh at sync time.
	rendered []string
	// bytes is the text the backlog holds, bounded by maxBacklogBytes.
	bytes int
	// cells mirrors rawLines with each line laid out for cellsKey, so a
	// batch lays out its own lines only. paintGen numbers the search
	// paint: a painted cell from an older generation is drawn again.
	cells    []cell
	cellsKey layoutKey
	paintGen uint64
	// rowStart[i] is the viewport row line i starts on, and its last entry
	// the row count: wrapped lines take several rows.
	rowStart []int
	width    int
	bodyH    int

	// podColors maps each pod to its palette colour, dealt in the order
	// the pods appear so neighbouring replicas land on distinct hues.
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

// logEvent is what the pump hands the view: a line with its stamp, or the
// end of one reader, with Err for one that failed. Attempt numbers the
// reader. An end without a pod is the merge itself failing.
type logEvent struct {
	Pod     string
	Attempt uint64
	Line    string
	TS      time.Time
	Err     error
	Done    bool
}

// logEventsBatch is the result of one drain tick: every event the channel
// had ready (up to drainBatchCap). tok names the session it was read from.
type logEventsBatch struct {
	tok    async.Token
	events []logEvent
}

// Option changes how New builds the view.
type Option func(*Model)

// WithTarget names the deployment or service the pods were resolved from,
// so the footer shows the selector command the stream stands for and the
// view follows the pods it selects.
func WithTarget(t k8s.LogTarget) Option {
	return func(m *Model) { m.target = t }
}

// WithClock arms the view's timers (the re-resolve, the debounce of a
// stream's end, reconnects) on c, for tests; the root never passes one.
func WithClock(c async.Clock) Option {
	return func(m *Model) { m.clock = c }
}

// New constructs a logs view and *immediately* starts the streaming
// goroutines. They are torn down on Close().
//
// The tail is per pod, and clamped so that all pods together start with no
// more than the buffer holds (views.ClampTail). At most maxStreams pods
// stream at once.
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
		clock:       async.RealClock{},
		namespace:   namespace,
		tail:        views.ClampTail(tail, views.StreamedPods(len(pods))),
		container:   container,
		viewport:    newViewport(),
		podColors:   assignPodColors(pods),
		multi:       len(pods) > 1,
		autoFollow:  true,
		paintGen:    noPaint + 1,
		searchInput: si,
		keys:        newKeyMap(),
	}
	if client != nil {
		m.src, m.res = client, client
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.mint = async.New().Next()
	return m.begin(pods).startPrevious()
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

// Init starts reading the first session's events, and whatever the view
// arranged when it was built: the first re-resolve, a pod's restart count
// to tell a restart by, the status of pods left out.
func (m Model) Init() tea.Cmd {
	return tea.Batch(waitOrDrainCmd(m.sess), m.startup)
}

// Update implements views.View.
func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case logEventsBatch:
		return m.onLogEventsBatch(msg)
	case async.TickMsg:
		return m.onFollowTick(msg)
	case async.Result[k8s.LogTarget]:
		return m.onReresolved(msg)
	case async.Result[[]podCheck]:
		return m.onChecked(msg)
	case explainDueMsg:
		return m.onExplainDue(msg)
	case retryMsg:
		return m.onRetry(msg)
	case recheckMsg:
		return m.onRecheck(msg)
	case baselineMsg:
		return m.onBaseline(msg)
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
