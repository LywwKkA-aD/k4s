package tuitest_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

// label is the only message the probe model reacts to.
type label string

// probe records every label it sees and answers with react's Cmd.
type probe struct {
	seen  []label
	react func(label) tea.Cmd
}

func (p probe) Init() tea.Cmd { return emit("init") }

func (p probe) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	l, ok := msg.(label)
	if !ok {
		return p, nil
	}
	p.seen = append(slices.Clone(p.seen), l)
	if p.react == nil {
		return p, nil
	}
	return p, p.react(l)
}

func (p probe) View() string {
	parts := make([]string, 0, len(p.seen))
	for _, l := range p.seen {
		parts = append(parts, string(l))
	}
	return strings.Join(parts, ",")
}

func emit(l label) tea.Cmd { return func() tea.Msg { return l } }

func reactions(m map[label]tea.Cmd) func(label) tea.Cmd {
	return func(l label) tea.Cmd { return m[l] }
}

func labels(msgs []tea.Msg) []label {
	return tuitest.All[label](msgs)
}

func TestDriveRunsCmdTrees(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		react    map[label]tea.Cmd
		inputs   []tea.Msg
		wantOut  []label
		wantSeen []label
	}{
		{
			name:     "produced messages are fed back",
			react:    map[label]tea.Cmd{"ping": emit("pong")},
			inputs:   []tea.Msg{label("ping")},
			wantOut:  []label{"pong"},
			wantSeen: []label{"ping", "pong"},
		},
		{
			name:     "batch children keep their order",
			react:    map[label]tea.Cmd{"go": tea.Batch(emit("a"), emit("b"), emit("c"))},
			inputs:   []tea.Msg{label("go")},
			wantOut:  []label{"a", "b", "c"},
			wantSeen: []label{"go", "a", "b", "c"},
		},
		{
			name: "sequences run in order and expand nested batches",
			react: map[label]tea.Cmd{"go": tea.Sequence(
				emit("a"), tea.Batch(emit("b1"), emit("b2")), emit("c"),
			)},
			inputs:   []tea.Msg{label("go")},
			wantOut:  []label{"a", "b1", "b2", "c"},
			wantSeen: []label{"go", "a", "b1", "b2", "c"},
		},
		{
			name:     "each input settles before the next",
			react:    map[label]tea.Cmd{"ping": emit("pong")},
			inputs:   []tea.Msg{label("ping"), label("x")},
			wantOut:  []label{"pong"},
			wantSeen: []label{"ping", "pong", "x"},
		},
		{
			name:     "nil messages are dropped",
			react:    map[label]tea.Cmd{"go": func() tea.Msg { return nil }},
			inputs:   []tea.Msg{label("go")},
			wantSeen: []label{"go"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, out := tuitest.Drive(t, probe{react: reactions(tc.react)}, tc.inputs...)
			if got := labels(out); !slices.Equal(got, tc.wantOut) {
				t.Errorf("emitted %v, want %v", got, tc.wantOut)
			}
			if got := m.seen; !slices.Equal(got, tc.wantSeen) {
				t.Errorf("model saw %v, want %v", got, tc.wantSeen)
			}
		})
	}
}

func TestDriveSkipsBlockingCmds(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	defer close(release)
	blocking := func() tea.Msg {
		<-release
		return label("late")
	}
	p := probe{react: reactions(map[label]tea.Cmd{"go": tea.Batch(blocking, emit("fast"))})}

	start := time.Now()
	_, out := tuitest.DriveWith(t, tuitest.Driver{CmdTimeout: 20 * time.Millisecond}, p, label("go"))
	if got := labels(out); !slices.Equal(got, []label{"fast"}) {
		t.Errorf("emitted %v, want only the fast message", got)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("drive took %v; the blocking Cmd was not abandoned", elapsed)
	}
}

func TestDriveStopsAtQuit(t *testing.T) {
	t.Parallel()

	p := probe{react: reactions(map[label]tea.Cmd{"stop": tea.Quit})}
	m, out := tuitest.Drive(t, p, label("stop"), label("after"))
	if len(out) != 1 || !tuitest.IsQuit(out[0]) {
		t.Fatalf("emitted %s, want one quit", tuitest.Describe(out))
	}
	if got := m.seen; !slices.Equal(got, []label{"stop"}) {
		t.Errorf("model saw %v after quit, want only stop", got)
	}
}

func TestDriveFailurePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		run       func(tb testing.TB)
		wantFatal string
		wantError string
	}{
		{
			name: "self re-arming cmd hits the message cap",
			run: func(tb testing.TB) {
				p := probe{react: reactions(map[label]tea.Cmd{"loop": emit("loop")})}
				tuitest.DriveWith(tb, tuitest.Driver{MaxMsgs: 5}, p, label("loop"))
			},
			wantFatal: "did not settle",
		},
		{
			name: "panicking cmd fails instead of crashing",
			run: func(tb testing.TB) {
				p := probe{react: reactions(map[label]tea.Cmd{"boom": func() tea.Msg { panic("kaboom") }})}
				tuitest.Drive(tb, p, label("boom"))
			},
			wantError: "kaboom",
		},
		{
			name: "update returning another type",
			run: func(tb testing.TB) {
				tuitest.Drive(tb, shapeShifter{}, label("x"))
			},
			wantFatal: "returned",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fatals, errs := record(tc.run).failures()
			if tc.wantFatal != "" && !anyContains(fatals, tc.wantFatal) {
				t.Errorf("fatals %q, want one containing %q", fatals, tc.wantFatal)
			}
			if tc.wantError != "" && !anyContains(errs, tc.wantError) {
				t.Errorf("errors %q, want one containing %q", errs, tc.wantError)
			}
		})
	}
}

// shapeShifter returns a different model type from Update.
type shapeShifter struct{}

func (shapeShifter) Init() tea.Cmd                       { return nil }
func (shapeShifter) Update(tea.Msg) (tea.Model, tea.Cmd) { return probe{}, nil }
func (shapeShifter) View() string                        { return "" }

func anyContains(list []string, sub string) bool {
	return slices.ContainsFunc(list, func(s string) bool { return strings.Contains(s, sub) })
}

func TestStepDoesNotFeedBack(t *testing.T) {
	t.Parallel()

	p := probe{react: reactions(map[label]tea.Cmd{"ping": emit("pong")})}
	m, out := tuitest.Step(t, p, label("ping"))
	if got := labels(out); !slices.Equal(got, []label{"pong"}) {
		t.Errorf("Step emitted %v, want [pong]", got)
	}
	if got := m.seen; !slices.Equal(got, []label{"ping"}) {
		t.Errorf("Step fed back: model saw %v", got)
	}
}

func TestInitDrivesTheInitCmd(t *testing.T) {
	t.Parallel()

	p := probe{react: reactions(map[label]tea.Cmd{"init": emit("ready")})}
	m, out := tuitest.Init(t, p)
	if got := labels(out); !slices.Equal(got, []label{"init", "ready"}) {
		t.Errorf("Init emitted %v, want [init ready]", got)
	}
	if got := tuitest.Screen(m); got != "init,ready" {
		t.Errorf("screen = %q, want init,ready", got)
	}
}

func TestCollectAndMessageHelpers(t *testing.T) {
	t.Parallel()

	out := tuitest.Collect(t, tea.Batch(emit("a"), tea.Quit, emit("b")))
	if got := labels(out); !slices.Equal(got, []label{"a", "b"}) {
		t.Errorf("All[label] = %v, want [a b]", got)
	}
	if first, ok := tuitest.Find[label](out); !ok || first != "a" {
		t.Errorf("Find[label] = %q, %v", first, ok)
	}
	if _, ok := tuitest.Find[int](out); ok {
		t.Error("Find[int] found a message that is not there")
	}
	if !tuitest.Contains(out, label("b")) || tuitest.Contains(out, label("z")) {
		t.Error("Contains does not compare by value")
	}
	if !strings.Contains(tuitest.Describe(out), "tuitest_test.label a") {
		t.Errorf("Describe = %q", tuitest.Describe(out))
	}
	if tuitest.Describe(nil) != "(no messages)" {
		t.Errorf("Describe(nil) = %q", tuitest.Describe(nil))
	}
	if got := tuitest.Collect(t, nil); got != nil {
		t.Errorf("Collect(nil) = %v", got)
	}
}

func TestRequestsKeepsCrossViewMessagesOnly(t *testing.T) {
	t.Parallel()

	describe := views.DescribeRequestMsg{Kind: "pod", Namespace: "demo", Name: "web-1"}
	got := tuitest.Requests([]tea.Msg{label("private"), describe, nil, views.NamespaceSelectedMsg{}})
	if len(got) != 2 || got[0] != describe {
		t.Errorf("Requests = %s", tuitest.Describe(got))
	}
}

func TestMountSizesThenStarts(t *testing.T) {
	t.Parallel()

	p := probe{react: reactions(map[label]tea.Cmd{"init": emit("ready")})}
	m, out := tuitest.Mount(t, p, 80, 24)
	if got := labels(out); !slices.Equal(got, []label{"init", "ready"}) {
		t.Errorf("Mount emitted %v", got)
	}
	if m.View() != "init,ready" {
		t.Errorf("model saw %q", m.View())
	}
	if n := len(tuitest.FilterBy("web")); n != 3 {
		t.Errorf("FilterBy returned %d keys, want open, text, commit", n)
	}
}

func TestIsExec(t *testing.T) {
	t.Parallel()

	execMsg := tea.ExecProcess(exec.Command("true"), nil)()
	if !tuitest.IsExec(execMsg) {
		t.Errorf("IsExec(%T) = false", execMsg)
	}
	if tuitest.IsExec(label("x")) || tuitest.IsQuit(label("x")) {
		t.Error("plain messages are neither exec nor quit")
	}
}
