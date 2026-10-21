package overlay_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/LywwKkA-aD/k4s/internal/tui/overlay"
	"github.com/LywwKkA-aD/k4s/internal/tui/tuitest"
	"github.com/LywwKkA-aD/k4s/internal/tui/views"
)

func serviceForward(hint uint16) views.ForwardRequestMsg {
	return views.ForwardRequestMsg{Kind: "service", Namespace: "demo", Name: "web", RemotePort: hint}
}

func allowAll(uint16) error { return nil }

func TestForwardPromptOpensPrefilled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		req  views.ForwardRequestMsg
		want []string
	}{
		{name: "privileged remote port gets a high local port", req: serviceForward(80), want: []string{"port-forward", "service/web · demo", "❯ 8080:80", "format local:remote"}},
		{name: "high remote port is kept", req: serviceForward(8443), want: []string{"❯ 8443:8443"}},
		{name: "no hint shows a placeholder", req: views.ForwardRequestMsg{Kind: "pod", Namespace: "demo", Name: "web-1"}, want: []string{"pod/web-1 · demo", "8080:80"}},
		{name: "no namespace", req: views.ForwardRequestMsg{Kind: "pod", Name: "web-1"}, want: []string{"pod/web-1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			requireShows(t, overlay.NewForward(tc.req, allowAll), tc.want...)
		})
	}
}

// retype clears the prompt and types text, if any.
func retype(text string) []tea.Msg {
	if text == "" {
		return []tea.Msg{tuitest.Press("ctrl+u")}
	}
	return []tea.Msg{tuitest.Press("ctrl+u"), tuitest.Type(text)}
}

func TestForwardPromptAccepts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		hint          uint16
		input         []tea.Msg
		local, remote uint16
	}{
		{name: "prefill", hint: 80, local: 8080, remote: 80},
		{name: "pair", hint: 80, input: retype("9000:81"), local: 9000, remote: 81},
		{name: "local only takes the hint", hint: 80, input: retype("9000"), local: 9000, remote: 80},
		{name: "local only without a hint", input: retype("9000"), local: 9000, remote: 9000},
		{name: "remote only", hint: 80, input: retype(":8443"), local: 8443, remote: 8443},
		{name: "local with a colon", hint: 80, input: retype("7000:"), local: 7000, remote: 7000},
		{name: "empty takes the hint", hint: 3000, input: retype(""), local: 3000, remote: 3000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var checked []uint16
			check := func(p uint16) error {
				checked = append(checked, p)
				return nil
			}
			input := append(slices.Clone(tc.input), tuitest.Enter())
			o, out := feed(t, overlay.NewForward(serviceForward(tc.hint), check), input...)
			if o != nil {
				t.Fatalf("still open:\n%s", screen(o))
			}
			want := overlay.ForwardChosenMsg{Request: serviceForward(tc.hint), Local: tc.local, Remote: tc.remote}
			if len(out) != 1 || !tuitest.Contains(out, want) {
				t.Fatalf("messages = %s, want %+v", tuitest.Describe(out), want)
			}
			if len(checked) != 1 || checked[0] != tc.local {
				t.Fatalf("checked ports %v, want [%d]", checked, tc.local)
			}
		})
	}
}

func TestForwardPromptKeepsBadInputOpen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		hint  uint16
		input string
		want  string
	}{
		{name: "empty without a hint", input: "", want: "port required (try 8080:80)"},
		{name: "not a number", hint: 80, input: "abc", want: `"abc" is not a port number`},
		{name: "zero", hint: 80, input: "0", want: "port must be > 0"},
		{name: "out of range", hint: 80, input: "70000", want: `"70000" is not a port number`},
		{name: "both sides empty", hint: 80, input: ":", want: "both ports empty"},
		{name: "bad local side", hint: 80, input: "x:80", want: `local port: "x" is not a port number`},
		{name: "bad remote side", hint: 80, input: "8080:y", want: `remote port: "y" is not a port number`},
		{name: "three parts", hint: 80, input: "1:2:3", want: `remote port: "2:3" is not a port number`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			input := append(retype(tc.input), tuitest.Enter())
			o, out := feed(t, overlay.NewForward(serviceForward(tc.hint), allowAll), input...)
			if len(out) != 0 {
				t.Fatalf("bad input answered: %s", tuitest.Describe(out))
			}
			requireShows(t, o, tc.want, "format local:remote")
		})
	}
}

func TestForwardPromptShowsABindError(t *testing.T) {
	t.Parallel()

	busy := func(p uint16) error {
		if p == 8080 {
			return errors.New("local port 8080 already in use — pick another")
		}
		return nil
	}
	o, out := feed(t, overlay.NewForward(serviceForward(80), busy), tuitest.Enter())
	if len(out) != 0 {
		t.Fatalf("a busy port answered: %s", tuitest.Describe(out))
	}
	requireShows(t, o, "local port 8080 already in use — pick another", "❯ 8080:80")

	// Editing the input clears the error; a free port then goes through.
	o, _ = feed(t, o, tuitest.Press("ctrl+u"), tuitest.Type("9090:80"))
	if got := screen(o); strings.Contains(got, "already in use") {
		t.Fatalf("stale error after editing:\n%s", got)
	}
	o, out = feed(t, o, tuitest.Enter())
	want := overlay.ForwardChosenMsg{Request: serviceForward(80), Local: 9090, Remote: 80}
	if o != nil || !tuitest.Contains(out, want) {
		t.Fatalf("free port: open=%v, messages %s", o != nil, tuitest.Describe(out))
	}
}

func TestForwardPromptWithoutACheckAccepts(t *testing.T) {
	t.Parallel()

	o, out := feed(t, overlay.NewForward(serviceForward(80), nil), tuitest.Enter())
	if o != nil || len(out) != 1 {
		t.Fatalf("open=%v, messages %s", o != nil, tuitest.Describe(out))
	}
}

func TestForwardPromptEscAndNarrow(t *testing.T) {
	t.Parallel()

	o := overlay.NewForward(serviceForward(80), allowAll)
	for width := 1; width <= 80; width++ {
		requireNarrow(t, o, width)
	}
	o, _ = feed(t, o, tuitest.Resize(80, 24))
	requireOpen(t, o)
	if o, out := feed(t, o, tuitest.Esc()); o != nil || len(out) != 0 {
		t.Fatalf("esc: open=%v, messages %s", o != nil, tuitest.Describe(out))
	}
}
