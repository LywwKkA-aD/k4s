package tui

import (
	"fmt"
	"image/color"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
)

// asksBackground reports whether msgs hold the program's background
// request, an unexported Bubble Tea message.
func asksBackground(msgs []tea.Msg) bool {
	return slices.ContainsFunc(msgs, func(m tea.Msg) bool {
		return fmt.Sprintf("%T", m) == "tea.backgroundColorMsg"
	})
}

func TestFirstFrameWaitsForTheBackground(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		msg  tea.Msg
	}{
		{name: "the terminal answers", msg: tea.BackgroundColorMsg{Color: color.White}},
		{name: "the terminal stays silent", msg: holdDoneMsg{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m, _ := tuitest.Drive(t, New(tuitest.NewFakeClient(t), nil, WithBackgroundQuery(true), instantFrames()), tuitest.Resize(80, 24))
			held := m.View()
			if held.Content != "" || !held.AltScreen {
				t.Fatalf("the first frame was drawn before the background was known: %q", held.Content)
			}
			if held.WindowTitle == "" {
				t.Error("the held frame dropped the window title")
			}
			next, _ := tuitest.Drive(t, m, tc.msg)
			if next.View().Content == "" {
				t.Fatal("the frame is still held")
			}
		})
	}
}

func TestInitAsksForTheBackgroundOnlyWhenTold(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts []Option
		want bool
	}{
		{name: "theme follows the terminal", opts: []Option{WithBackgroundQuery(true)}, want: true},
		{name: "fixed theme", opts: []Option{WithBackgroundQuery(false)}, want: false},
		{name: "no option", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := New(tuitest.NewFakeClient(t), nil, tc.opts...)
			out := tuitest.Collect(t, m.Init())
			if got := asksBackground(out); got != tc.want {
				t.Fatalf("asks for the background = %v, want %v: %s", got, tc.want, tuitest.Describe(out))
			}
			if _, deadline := tuitest.Find[holdDoneMsg](out); deadline != tc.want {
				t.Errorf("hold deadline armed = %v, want %v", deadline, tc.want)
			}
			if tc.want == (m.View().Content != "") {
				t.Errorf("first frame drawn = %v, want %v", m.View().Content != "", !tc.want)
			}
		})
	}
}

// Started the way the program starts it, the held frame gives way within
// the hold.
func TestMountedModelShowsAfterTheHold(t *testing.T) {
	t.Parallel()

	m, _ := tuitest.Mount(t, New(tuitest.NewFakeClient(t), nil, WithBackgroundQuery(true), instantFrames()), 80, 24)
	if m.View().Content == "" {
		t.Fatal("the frame is still held after Init settled")
	}
}
