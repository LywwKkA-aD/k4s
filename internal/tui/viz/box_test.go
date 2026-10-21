package viz_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/LywwKkA-aD/k4s/internal/tui/viz"
)

func TestBoxEdges(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		box   viz.Box
		width int
		want  string
	}{
		{
			name: "title info footer", box: viz.Box{Title: "pods", Info: "3", Footer: "ok"}, width: 20,
			want: "╭─┤ pods ├───── 3 ─╮\n│                  │\n╰───────────── ok ─╯",
		},
		{
			name: "focused is heavy", box: viz.Box{Title: "pods", Info: "3", Footer: "ok", Focused: true}, width: 20,
			want: "┏━┫ pods ┣━━━━━ 3 ━┓\n┃                  ┃\n┗━━━━━━━━━━━━━ ok ━┛",
		},
		{
			name: "no title", box: viz.Box{Info: "3"}, width: 10,
			want: "╭──── 3 ─╮\n│        │\n╰────────╯",
		},
		{
			name: "info dropped before the title", box: viz.Box{Title: "pods", Info: "185 · 33 ns"}, width: 14,
			want: "╭─┤ pods ├───╮\n│            │\n╰────────────╯",
		},
		{
			name: "info cut when some fits", box: viz.Box{Title: "pods", Info: "185 · 33 ns"}, width: 18,
			want: "╭─┤ pods ├─ 18… ─╮\n│                │\n╰────────────────╯",
		},
		{
			name: "title cut", box: viz.Box{Title: "workloads"}, width: 11,
			want: "╭─┤ wor… ├╮\n│         │\n╰─────────╯",
		},
		{
			name: "title dropped when nothing fits", box: viz.Box{Title: "workloads"}, width: 7,
			want: "╭─────╮\n│     │\n╰─────╯",
		},
		{
			name: "footer cut", box: viz.Box{Footer: "7 need attention"}, width: 12,
			want: "╭──────────╮\n│          │\n╰─ 7 nee… ─╯",
		},
		{
			name: "two columns", box: viz.Box{Title: "x"}, width: 2,
			want: "╭╮\n││\n╰╯",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := plain(c.box.Render(viz.Mono(), c.width, 3, "")); got != c.want {
				t.Errorf("Render:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func TestBoxBody(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		height int
		body   string
		want   []string
	}{
		{name: "cut with ellipsis", height: 3, body: "abcdefghijk", want: []string{"│abcdefg…│"}},
		{name: "padded", height: 3, body: " ok", want: []string{"│ ok     │"}},
		{name: "missing lines are blank", height: 4, body: "a", want: []string{"│a       │", "│        │"}},
		{name: "lines past the height are dropped", height: 3, body: "a\nb\nc", want: []string{"│a       │"}},
		{name: "no body rows", height: 2, body: "a", want: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			lines := strings.Split(plain(viz.Box{}.Render(viz.Mono(), 10, c.height, c.body)), "\n")
			got := lines[1 : len(lines)-1]
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("body rows %q, want %q", got, c.want)
			}
		})
	}
}

func TestBoxKeepsStyledBodyLines(t *testing.T) {
	t.Parallel()
	styled := colour().Err.Render("▲ CrashLoopBackOff for a long while")
	out := viz.Box{}.Render(colour(), 12, 3, styled)
	assertBlock(t, out, 12, 3)
	row := strings.Split(out, "\n")[1]
	if got := plain(row); got != "│▲ CrashLo…│" {
		t.Errorf("styled row = %q", got)
	}
	if !strings.Contains(row, "\x1b[") {
		t.Errorf("styled row lost its escape codes: %q", row)
	}
}

func TestBoxDegenerateSizes(t *testing.T) {
	t.Parallel()
	b := viz.Box{Title: "pods", Info: "3"}
	if got := plain(b.Render(viz.Mono(), 12, 1, "body")); got != "╭─┤ pods ├─╮" {
		t.Errorf("height 1 = %q, want the top edge", got)
	}
	if got := b.Render(viz.Mono(), 1, 3, "body"); got != " \n \n " {
		t.Errorf("width 1 = %q, want blanks", got)
	}
}

// Focus is a heavy border and a bold title in every theme, so mono needs
// no colour to show it.
func TestBoxFocusInMono(t *testing.T) {
	t.Parallel()
	focused := viz.Box{Title: "pods", Focused: true}.Render(viz.Mono(), 20, 3, "")
	if !strings.Contains(focused, "\x1b[1m") {
		t.Errorf("focused title not bold in mono: %q", focused)
	}
	plainBox := viz.Box{Title: "pods"}.Render(viz.Mono(), 20, 3, "")
	if plainBox != ansi.Strip(plainBox) {
		t.Errorf("unfocused box in mono should carry no attributes: %q", plainBox)
	}
}
