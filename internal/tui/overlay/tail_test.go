package overlay_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func deployTail() views.TailPromptRequestMsg {
	return views.TailPromptRequestMsg{
		Namespace: "app",
		Pods:      []string{"app-1", "app-2"},
		Container: "web",
		Target:    views.ResourceRef{Kind: "deployment", Namespace: "app", Name: "app"},
	}
}

func TestTailPromptAnswers(t *testing.T) {
	t.Parallel()

	clear := tuitest.Press("ctrl+u")
	cases := []struct {
		name  string
		input []tea.Msg
		want  int64 // 0: cancelled
	}{
		{name: "enter takes the default", input: keys("enter"), want: 100},
		{name: "custom count", input: []tea.Msg{clear, tuitest.Type("250"), tuitest.Enter()}, want: 250},
		{name: "surrounding blanks are fine", input: []tea.Msg{clear, tuitest.Type(" 30 "), tuitest.Enter()}, want: 30},
		{name: "garbage falls back to the default", input: []tea.Msg{clear, tuitest.Type("lots"), tuitest.Enter()}, want: 100},
		{name: "zero falls back to the default", input: []tea.Msg{clear, tuitest.Type("0"), tuitest.Enter()}, want: 100},
		{name: "negative falls back to the default", input: []tea.Msg{clear, tuitest.Type("-5"), tuitest.Enter()}, want: 100},
		{name: "empty falls back to the default", input: []tea.Msg{clear, tuitest.Enter()}, want: 100},
		{name: "a count over the cap is clamped", input: []tea.Msg{clear, tuitest.Type("100000"), tuitest.Enter()}, want: 2500},
		{name: "esc cancels", input: keys("esc")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, out := feed(t, overlay.NewTail(deployTail()), tc.input...)
			if o != nil {
				t.Fatalf("still open:\n%s", screen(o))
			}
			if tc.want == 0 {
				if len(out) != 0 {
					t.Fatalf("cancel answered: %s", tuitest.Describe(out))
				}
				return
			}
			want := overlay.TailChosenMsg{Request: deployTail(), Tail: tc.want}
			if len(out) != 1 || !tuitest.Contains(out, want) {
				t.Fatalf("messages = %s, want %+v", tuitest.Describe(out), want)
			}
		})
	}
}

func TestTailPromptNamesItsTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  views.TailPromptRequestMsg
		want string
	}{
		{name: "deployment row", req: deployTail(), want: "tail lines · deployment/app · ns app"},
		{
			name: "service row",
			req:  views.TailPromptRequestMsg{Namespace: "shop", Pods: []string{"a", "b"}, Target: views.ResourceRef{Kind: "service", Namespace: "shop", Name: "api"}},
			want: "tail lines · service/api · ns shop",
		},
		{name: "one pod without a target", req: views.TailPromptRequestMsg{Namespace: "demo", Pods: []string{"web-1"}}, want: "tail lines · pod/web-1 · ns demo"},
		{name: "several pods without a target", req: views.TailPromptRequestMsg{Namespace: "demo", Pods: []string{"web-1", "web-2"}}, want: "tail lines · 2 pods · ns demo"},
		{name: "no namespace", req: views.TailPromptRequestMsg{Pods: []string{"web-1"}}, want: "tail lines · pod/web-1"},
		{name: "target without a kind", req: views.TailPromptRequestMsg{Target: views.ResourceRef{Namespace: "x", Name: "thing"}}, want: "tail lines · thing · ns x"},
		{name: "target namespace wins", req: views.TailPromptRequestMsg{Namespace: "", Target: views.ResourceRef{Kind: "pod", Namespace: "x", Name: "p"}}, want: "tail lines · pod/p · ns x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o := overlay.NewTail(tc.req)
			title, _, _ := strings.Cut(screen(o), "\n")
			if got := strings.TrimSpace(title); got != tc.want {
				t.Fatalf("title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTailPromptView(t *testing.T) {
	t.Parallel()

	o := overlay.NewTail(deployTail())
	requireShows(t, o, "❯ 100", "Enter run · Esc cancel · default 100", "max 2500 per pod, 5000 in all")
	o, _ = feed(t, o, tuitest.Press("q"), tuitest.Press(":"))
	requireShows(t, o, "❯ 100q:")
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestTailPromptIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	o, _ := feed(t, overlay.NewTail(deployTail()), tuitest.Resize(80, 24))
	requireShows(t, o, "❯ 100")
}

func TestTailPromptNamesTheCap(t *testing.T) {
	t.Parallel()

	pods := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "web-" + string(rune('a'+i))
		}
		return out
	}
	cases := []struct {
		pods int
		want string
	}{
		{pods: 1, want: "max 5000 lines"},
		{pods: 2, want: "max 2500 per pod, 5000 in all"},
		{pods: 20, want: "max 250 per pod, 5000 in all"},
	}
	for _, tc := range cases {
		o := overlay.NewTail(views.TailPromptRequestMsg{Namespace: "demo", Pods: pods(tc.pods)})
		requireShows(t, o, tc.want)
	}
}
