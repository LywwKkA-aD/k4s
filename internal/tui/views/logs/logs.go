// Package logs is the streaming logs view: per-pod goroutines feed lines into
// a shared channel; the view appends them to a viewport with a coloured
// per-pod prefix so multiple replicas are visually distinguishable.
package logs

import (
	"bufio"
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/key"
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
	scannerBufferSize = 1024 * 1024 // 1 MiB max log line
	eventBufferSize   = 256
)

// Model is the streaming logs view.
type Model struct {
	client    *k8s.Client
	namespace string
	pods      []string

	viewport viewport.Model
	lines    []string
	width    int

	events chan logEvent
	cancel context.CancelFunc

	autoFollow bool

	pauseKey  key.Binding
	clearKey  key.Binding
	scrollKey key.Binding
}

type logEvent struct {
	Pod  string
	Line string
	Err  error
	Done bool // stream ended (clean or with err)
}

type streamsClosedMsg struct{}

// New constructs a logs view and *immediately* starts the streaming
// goroutines. They are torn down on Close(). The shared events channel is
// closed once every pod's goroutine returns, which signals streamsClosedMsg
// to the bubbletea loop and prevents leaked Cmd reads.
func New(client *k8s.Client, namespace string, pods []string) Model {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan logEvent, eventBufferSize)

	if client != nil && len(pods) > 0 {
		var wg sync.WaitGroup
		for _, pod := range pods {
			wg.Add(1)
			go func(p string) {
				defer wg.Done()
				streamOnePod(ctx, client, namespace, p, events)
			}(pod)
		}
		go func() {
			wg.Wait()
			close(events)
		}()
	} else {
		close(events)
	}

	vp := viewport.New(80, 20)

	return Model{
		client:     client,
		namespace:  namespace,
		pods:       pods,
		viewport:   vp,
		events:     events,
		cancel:     cancel,
		autoFollow: true,
		pauseKey: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause/follow"),
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
func streamOnePod(ctx context.Context, client *k8s.Client, ns, pod string, ch chan<- logEvent) {
	stream, err := client.StreamPodLogs(ctx, ns, pod, defaultTailLines)
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
	// Treat scanner errors that happen because we cancelled the context as
	// a clean close — there's nothing the user needs to see.
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		send(ctx, ch, logEvent{Pod: pod, Err: err, Done: true})
		return
	}
	send(ctx, ch, logEvent{Pod: pod, Done: true})
}

// send pushes ev to ch unless ctx is already cancelled. Returns false if the
// caller should stop (ctx cancelled).
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

// Update handles size, key bindings, and incoming log events.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.viewport.Width = msg.Width
		m.viewport.Height = max(msg.Height, minViewportHeight)
		m.refreshContent()

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.pauseKey):
			m.autoFollow = !m.autoFollow
			return m, nil
		case key.Matches(msg, m.clearKey):
			m.lines = nil
			m.viewport.SetContent("")
			return m, nil
		}

	case logEvent:
		switch {
		case msg.Err != nil:
			m.appendLine(styles.Warn.Render(fmt.Sprintf("[%s] stream error: %v", msg.Pod, msg.Err)))
		case msg.Done:
			m.appendLine(styles.Hint.Render(fmt.Sprintf("[%s] stream closed", msg.Pod)))
		default:
			m.appendLine(formatLine(msg.Pod, msg.Line, len(m.pods) > 1))
		}
		m.refreshContent()
		return m, waitForEventCmd(m.events)

	case streamsClosedMsg:
		// All goroutines done, channel closed. Stop pumping.
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m *Model) appendLine(line string) {
	m.lines = append(m.lines, line)
	if len(m.lines) > maxBufferedLines {
		m.lines = m.lines[len(m.lines)-maxBufferedLines:]
	}
}

func (m *Model) refreshContent() {
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	if m.autoFollow {
		m.viewport.GotoBottom()
	}
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

// View renders the viewport or a placeholder.
func (m Model) View() string {
	if m.client == nil {
		return styles.Warn.Render("no kubeconfig")
	}
	if len(m.lines) == 0 {
		return styles.Hint.Render(fmt.Sprintf("waiting for logs… (tail %d)", defaultTailLines))
	}
	status := ""
	if !m.autoFollow {
		status = styles.Warn.Render("[paused] ") + "\n"
	}
	return status + m.viewport.View()
}

// Title implements views.View.
func (m Model) Title() string {
	if len(m.pods) == 1 {
		return "logs · " + m.pods[0]
	}
	return fmt.Sprintf("logs · %d pods", len(m.pods))
}

// KubectlEquivalent implements views.View.
func (m Model) KubectlEquivalent() string {
	if len(m.pods) == 1 {
		return fmt.Sprintf("kubectl logs -f %s -n %s --tail=%d", m.pods[0], m.namespace, defaultTailLines)
	}
	return fmt.Sprintf("kubectl logs -f --tail=%d -n %s {%s}", defaultTailLines, m.namespace, strings.Join(m.pods, ","))
}

// Help implements views.View.
func (m Model) Help() []key.Binding {
	return []key.Binding{m.scrollKey, m.pauseKey, m.clearKey}
}

// Close cancels the streaming context, which unblocks the goroutines, drains
// them, and ultimately closes the events channel. Idempotent.
func (m Model) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}
