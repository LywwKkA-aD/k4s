package overlay_test

import (
	"strings"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func pickerRequest() views.ContainerPromptRequestMsg {
	return views.ContainerPromptRequestMsg{
		Namespace:  "demo",
		Pods:       []string{"multi-0"},
		Containers: []string{"app", "sidecar", "init"},
		NextKind:   "logs",
		Target:     views.ResourceRef{Kind: "pod", Namespace: "demo", Name: "multi-0"},
	}
}

func TestContainerPicker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		keys []string
		want string // "" with closed: cancelled; "" open: still picking
		open bool
	}{
		{name: "enter picks the first", keys: []string{"enter"}, want: "app"},
		{name: "down moves", keys: []string{"down", "enter"}, want: "sidecar"},
		{name: "j moves", keys: []string{"j", "j", "enter"}, want: "init"},
		{name: "up wraps to the last", keys: []string{"up", "enter"}, want: "init"},
		{name: "k wraps to the last", keys: []string{"k", "enter"}, want: "init"},
		{name: "down wraps to the first", keys: []string{"down", "down", "down", "enter"}, want: "app"},
		{name: "digit picks at once", keys: []string{"2"}, want: "sidecar"},
		{name: "digit out of range is ignored", keys: []string{"9"}, open: true},
		{name: "zero is ignored", keys: []string{"0"}, open: true},
		{name: "other keys are ignored", keys: []string{"x", "q", "?"}, open: true},
		{name: "esc cancels", keys: []string{"esc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, out := feed(t, overlay.NewContainer(pickerRequest()), keys(tc.keys...)...)
			if tc.open {
				requireOpen(t, o)
				return
			}
			if o != nil {
				t.Fatalf("still open:\n%s", screen(o))
			}
			if tc.want == "" {
				if len(out) != 0 {
					t.Fatalf("cancel answered: %s", tuitest.Describe(out))
				}
				return
			}
			want := overlay.ContainerChosenMsg{Request: pickerRequest(), Container: tc.want}
			if len(out) != 1 || !tuitest.Contains(out, want) {
				t.Fatalf("messages = %s, want %+v", tuitest.Describe(out), want)
			}
		})
	}
}

func TestContainerPickerView(t *testing.T) {
	t.Parallel()

	o := overlay.NewContainer(pickerRequest())
	title, _, _ := strings.Cut(screen(o), "\n")
	if got := strings.TrimSpace(title); got != "container · pod/multi-0 · ns demo" {
		t.Errorf("title = %q", got)
	}
	requireShows(t, o, "▶ 1. app", "  2. sidecar", "  3. init", "1-9 quick")
	o, _ = feed(t, o, keys("down")...)
	requireShows(t, o, "  1. app", "▶ 2. sidecar")
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
}

func TestContainerPickerWithoutContainersCloses(t *testing.T) {
	t.Parallel()

	o, out := feed(t, overlay.NewContainer(views.ContainerPromptRequestMsg{Pods: []string{"p"}}), tuitest.Press("j"))
	if o != nil || len(out) != 0 {
		t.Fatalf("empty picker: open=%v, messages %s", o != nil, tuitest.Describe(out))
	}
}

func TestContainerPickerIgnoresOtherMessages(t *testing.T) {
	t.Parallel()

	o, _ := feed(t, overlay.NewContainer(pickerRequest()), tuitest.Paste("2"), tuitest.Resize(80, 24))
	requireShows(t, o, "▶ 1. app")
}

func TestContainerPickerMarksADetachedTarget(t *testing.T) {
	t.Parallel()

	req := pickerRequest()
	req.Detached = true
	o := overlay.NewContainer(req)
	requireShows(t, o, "container · pod/multi-0 · ns demo", "(not the selected row)", "▶ 1. app")
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
	if strings.Contains(screen(overlay.NewContainer(pickerRequest())), "not the selected row") {
		t.Error("an attached request is marked")
	}
}
