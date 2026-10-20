package styles

import (
	"io"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// colourRenderer returns a renderer that emits colour codes. A test
// binary's stdout is not a TTY, so lipgloss would otherwise choose the
// Ascii profile, strip every code and let a "no colour" check pass
// vacuously. CLICOLOR_FORCE lifts that to the 16-colour profile, which is
// enough to show any colour a style asks for.
func colourRenderer(t *testing.T) *lipgloss.Renderer {
	t.Helper()
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	r := lipgloss.NewRenderer(io.Discard)
	r.SetHasDarkBackground(true)
	return r
}

// sample renders every style the views use, bound to r.
func sample(r *lipgloss.Renderer) map[string]string {
	tbl := Table()
	out := map[string]string{
		"title":        Title.Renderer(r).Render("k4s"),
		"subtitle":     Subtitle.Renderer(r).Render("sub"),
		"hint":         Hint.Renderer(r).Render("q quit"),
		"ok":           OK.Renderer(r).Render("ok"),
		"warn":         Warn.Renderer(r).Render("warn"),
		"err":          Err.Renderer(r).Render("err"),
		"stat":         Stat.Renderer(r).Render("3 pods"),
		"header":       Header.Renderer(r).Render("header"),
		"footer":       Footer.Renderer(r).Render("footer"),
		"kubectl":      KubectlHint.Renderer(r).Render("kubectl get pods"),
		"highlight":    Highlight.Renderer(r).Render("match"),
		"popup box":    PopupBox.Renderer(r).Render("popup"),
		"popup title":  PopupTitle.Renderer(r).Render("title"),
		"placeholder":  Placeholder.Renderer(r).Render("pods, ns"),
		"table header": tbl.Header.Renderer(r).Render("NAME"),
		"table cell":   tbl.Cell.Renderer(r).Render("nginx"),
		"selected row": tbl.Selected.Renderer(r).Render("nginx"),
	}
	for _, id := range PodPalette() {
		out["pod "+string(id)] = r.NewStyle().Foreground(PodColor(id)).Render("▌")
	}
	return out
}

var sgrPattern = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// sgrCodes returns the SGR parameters in s, folding the arguments of an
// extended 38/48 colour into the introducer so "38;5;1" does not read as
// bold.
func sgrCodes(s string) []int {
	var codes []int
	for _, m := range sgrPattern.FindAllStringSubmatch(s, -1) {
		params := strings.Split(m[1], ";")
		for i := 0; i < len(params); i++ {
			n, err := strconv.Atoi(params[i])
			if err != nil {
				continue
			}
			codes = append(codes, n)
			if (n == 38 || n == 48) && i+1 < len(params) {
				i += extendedArgs(params[i+1])
			}
		}
	}
	return codes
}

func extendedArgs(kind string) int {
	switch kind {
	case "5":
		return 2
	case "2":
		return 4
	}
	return 0
}

func isColourCode(n int) bool {
	return (n >= 30 && n <= 38) || (n >= 40 && n <= 48) ||
		(n >= 90 && n <= 97) || (n >= 100 && n <= 107)
}

func colourCodes(s string) []int {
	var out []int
	for _, n := range sgrCodes(s) {
		if isColourCode(n) {
			out = append(out, n)
		}
	}
	return out
}

func TestSGRCodesParsesExtendedColours(t *testing.T) {
	t.Parallel()
	got := sgrCodes("\x1b[1;38;5;212;48;2;1;7;4mX\x1b[0m")
	want := []int{1, 38, 48, 0}
	if !slices.Equal(got, want) {
		t.Errorf("sgrCodes = %v, want %v", got, want)
	}
}

// Guards against the vacuous pass: the forced renderer must really emit
// colour for the colour theme, or the mono check below proves nothing.
func TestColourThemeEmitsColourCodes(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	use(colourPalette)
	r := colourRenderer(t)
	for name, out := range sample(r) {
		if name == "header" || name == "footer" || name == "table cell" {
			continue // border-only or uncoloured on purpose
		}
		if len(colourCodes(out)) == 0 {
			t.Errorf("%s: colour theme rendered no colour: %q", name, out)
		}
	}
}

func TestMonoEmitsNoColourCodes(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	use(monoPalette)
	r := colourRenderer(t)
	for name, out := range sample(r) {
		if codes := colourCodes(out); len(codes) > 0 {
			t.Errorf("%s: mono rendered colour codes %v in %q", name, codes, out)
		}
		if strings.Contains(out, "38;") || strings.Contains(out, "48;") {
			t.Errorf("%s: mono rendered an extended colour in %q", name, out)
		}
	}
}

// Without colour, state must still show through attributes: reverse for
// the selected row and search match, bold for warnings, bold and
// underline for errors.
func TestMonoKeepsStateVisible(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	use(monoPalette)
	r := colourRenderer(t)
	out := sample(r)
	cases := []struct {
		name string
		want []int
	}{
		{name: "selected row", want: []int{7}},
		{name: "highlight", want: []int{7}},
		{name: "warn", want: []int{1}},
		{name: "ok", want: []int{1}},
		{name: "err", want: []int{1, 4}},
		{name: "title", want: []int{1}},
	}
	for _, c := range cases {
		codes := sgrCodes(out[c.name])
		for _, w := range c.want {
			if !slices.Contains(codes, w) {
				t.Errorf("%s: want SGR %d in %q", c.name, w, out[c.name])
			}
		}
	}
	if slices.Contains(sgrCodes(out["warn"]), 4) {
		t.Error("warn must not underline, or it reads as an error")
	}
	if slices.Contains(sgrCodes(out["hint"]), 2) {
		t.Error("hint must not be faint")
	}
}

func TestMonoProfile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tty  bool
		term string
		want termenv.Profile
	}{
		{name: "terminal keeps attributes", tty: true, term: "xterm-256color", want: termenv.ANSI},
		{name: "unset TERM on a terminal", tty: true, term: "", want: termenv.ANSI},
		{name: "dumb terminal", tty: true, term: "dumb", want: termenv.Ascii},
		{name: "not a terminal", tty: false, term: "xterm-256color", want: termenv.Ascii},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := monoProfile(tc.tty, tc.term); got != tc.want {
				t.Fatalf("monoProfile(%v, %q) = %v, want %v", tc.tty, tc.term, got, tc.want)
			}
		})
	}
}

// NO_COLOR is how most people reach mono, and termenv maps it to the Ascii
// profile, which strips reverse, bold and underline along with colour.
// Changes global lipgloss state, so it does not run in parallel.
func TestNoColorKeepsStateMarkers(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	prevTTY, prevProfile := stdoutIsTerminal, lipgloss.ColorProfile()
	stdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() {
		stdoutIsTerminal = prevTTY
		lipgloss.SetColorProfile(prevProfile)
		use(colourPalette)
	})

	if err := Init(os.Getenv); err != nil {
		t.Fatalf("Init: %v", err)
	}
	for name, s := range map[string]string{
		"selected row": Table().Selected.Render("nginx"),
		"warn":         Warn.Render("warn"),
		"err":          Err.Render("err"),
	} {
		codes := sgrCodes(s)
		if !slices.ContainsFunc(codes, func(c int) bool { return c == 1 || c == 4 || c == 7 }) {
			t.Errorf("%s %q has no bold, underline or reverse under NO_COLOR", name, s)
		}
		if slices.ContainsFunc(codes, func(c int) bool { return c == 38 || c == 48 }) {
			t.Errorf("%s %q carries a colour code under NO_COLOR", name, s)
		}
	}
}
