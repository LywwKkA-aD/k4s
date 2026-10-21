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
	"context"
	"fmt"
	"sync"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/k8s"
	"github.com/LywwKkA-aD/k4s/internal/kubectl"
	"github.com/LywwKkA-aD/k4s/internal/tui/styles"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

const (
	minViewportHeight = 5
	maxBufferedLines  = 5000
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
	// target is the deployment or service the pods were resolved from,
	// zero for a single pod; Command shows its selector.
	target k8s.LogTarget

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
func New(client *k8s.Client, namespace string, pods []string, tail int64, container string, opts ...Option) Model {
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
			streamOnePod(ctx, client, namespace, pods[0], k8s.LogOptions{Container: container, Tail: tail}, events)
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
	si.SetWidth(40)
	si.SetStyles(styles.TextInput())

	m := Model{
		client:      client,
		namespace:   namespace,
		pods:        pods,
		tail:        tail,
		container:   container,
		viewport:    newViewport(),
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
			key.WithKeys("up", "down", "pgup", "pgdown"),
			key.WithHelp("↑↓/pgup/pgdn", "scroll"),
		),
	}
	for _, opt := range opts {
		opt(&m)
	}
	return m
}

// newViewport returns the log viewport. Panning belongs to the view's own
// ←/→ hscroll, so the viewport's h/l panning stays off.
func newViewport() viewport.Model {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SetHorizontalStep(0)
	return vp
}

func (m Model) Init() tea.Cmd {
	return waitOrDrainCmd(m.events)
}

func (m Model) Update(msg tea.Msg) (views.View, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onWindowSize(msg)
	case logEventsBatch:
		return m.onLogEventsBatch(msg)
	case streamsClosedMsg:
		return m, nil
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

// Command implements views.View.
func (m Model) Command() kubectl.Cmd {
	return views.LogsCommand(m.namespace, m.pods, m.container, m.tail, m.target)
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
