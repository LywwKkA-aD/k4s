package styles

import (
	"bytes"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
)

// shown returns s as a terminal with profile p receives it. Style.Render
// always emits full-colour escape codes and the program downsamples them on
// output, so every check of what reaches the screen goes through a profile.
func shown(t *testing.T, p colorprofile.Profile, s string) string {
	t.Helper()
	var buf bytes.Buffer
	w := &colorprofile.Writer{Forward: &buf, Profile: p}
	if _, err := w.WriteString(s); err != nil {
		t.Fatalf("downsample to %v: %v", p, err)
	}
	return buf.String()
}

// sample renders every style the views use.
func sample() map[string]string {
	tbl := Table()
	out := map[string]string{
		"title":        Title.Render("k4s"),
		"subtitle":     Subtitle.Render("sub"),
		"hint":         Hint.Render("q quit"),
		"ok":           OK.Render("ok"),
		"warn":         Warn.Render("warn"),
		"err":          Err.Render("err"),
		"stat":         Stat.Render("3 pods"),
		"header":       Header.Render("header"),
		"footer":       Footer.Render("footer"),
		"kubectl":      KubectlHint.Render("kubectl get pods"),
		"highlight":    Highlight.Render("match"),
		"popup box":    PopupBox.Render("popup"),
		"popup title":  PopupTitle.Render("title"),
		"placeholder":  Placeholder.Render("pods, ns"),
		"table header": tbl.Header.Render("NAME"),
		"table cell":   tbl.Cell.Render("nginx"),
		"selected row": tbl.Selected.Render("nginx"),
	}
	for _, id := range PodPalette() {
		out["pod "+string(id)] = lipgloss.NewStyle().Foreground(PodColor(id)).Render("▌")
	}
	return out
}

var sgrPattern = regexp.MustCompile("\x1b\\[([0-9;:]*)m")

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

// Guards against the vacuous pass: the colour theme must still show colour
// on every colour profile, or the mono checks below prove nothing.
func TestColourThemeEmitsColourCodes(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	for _, p := range []colorprofile.Profile{colorprofile.ANSI, colorprofile.ANSI256, colorprofile.TrueColor} {
		for name, out := range sample() {
			if name == "header" || name == "footer" || name == "table cell" {
				continue // border-only or uncoloured on purpose
			}
			if len(colourCodes(shown(t, p, out))) == 0 {
				t.Errorf("%v %s: colour theme rendered no colour: %q", p, name, out)
			}
		}
	}
}

// TrueColor passes every code through, so a colour here would come from
// the mono styles themselves.
func TestMonoEmitsNoColourCodes(t *testing.T) {
	t.Cleanup(resetTheme)
	use(monoPalette)
	for name, out := range sample() {
		got := shown(t, colorprofile.TrueColor, out)
		if codes := colourCodes(got); len(codes) > 0 {
			t.Errorf("%s: mono rendered colour codes %v in %q", name, codes, got)
		}
		if strings.Contains(got, "38;") || strings.Contains(got, "48;") {
			t.Errorf("%s: mono rendered an extended colour in %q", name, got)
		}
	}
}

// Without colour, state must still show through attributes: reverse for
// the selected row and search match, bold for warnings, bold and
// underline for errors. They must survive the profile mono runs under on
// a terminal.
func TestMonoKeepsStateVisible(t *testing.T) {
	t.Cleanup(resetTheme)
	use(monoPalette)
	profile := monoProfile(true, "xterm-256color")
	out := sample()
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
		got := shown(t, profile, out[c.name])
		codes := sgrCodes(got)
		for _, w := range c.want {
			if !slices.Contains(codes, w) {
				t.Errorf("%s: want SGR %d in %q", c.name, w, got)
			}
		}
	}
	if slices.Contains(sgrCodes(shown(t, profile, out["warn"])), 4) {
		t.Error("warn must not underline, or it reads as an error")
	}
	if slices.Contains(sgrCodes(shown(t, profile, out["hint"])), 2) {
		t.Error("hint must not be faint")
	}
}

func TestMonoProfile(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		tty  bool
		term string
		want colorprofile.Profile
	}{
		{name: "terminal keeps attributes", tty: true, term: "xterm-256color", want: colorprofile.ASCII},
		{name: "unset TERM on a terminal", tty: true, term: "", want: colorprofile.ASCII},
		{name: "dumb terminal", tty: true, term: "dumb", want: colorprofile.NoTTY},
		{name: "not a terminal", tty: false, term: "xterm-256color", want: colorprofile.NoTTY},
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

// NO_COLOR is how most people reach mono. Init must pin a profile that
// drops colour but keeps reverse, bold and underline, and the styles must
// carry them. Changes package state, so it does not run in parallel.
func TestNoColorKeepsStateMarkers(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "xterm-256color")
	prevTTY := stdoutIsTerminal
	stdoutIsTerminal = func() bool { return true }
	t.Cleanup(func() {
		stdoutIsTerminal = prevTTY
		resetTheme()
	})

	if err := Init(os.Getenv); err != nil {
		t.Fatalf("Init: %v", err)
	}
	profile, forced := ColorProfile()
	if !forced || profile != colorprofile.ASCII {
		t.Fatalf("ColorProfile() = %v, %v; want ASCII, true", profile, forced)
	}
	for name, s := range map[string]string{
		"selected row": Table().Selected.Render("nginx"),
		"warn":         Warn.Render("warn"),
		"err":          Err.Render("err"),
	} {
		got := shown(t, profile, s)
		codes := sgrCodes(got)
		if !slices.ContainsFunc(codes, func(c int) bool { return c == 1 || c == 4 || c == 7 }) {
			t.Errorf("%s %q has no bold, underline or reverse under NO_COLOR", name, got)
		}
		if slices.ContainsFunc(codes, func(c int) bool { return c == 38 || c == 48 }) {
			t.Errorf("%s %q carries a colour code under NO_COLOR", name, got)
		}
	}
}

// The text input styles replace the bubbles defaults, which paint the
// prompt and cursor in ANSI colours; only the placeholder may be coloured.
func TestTextInputStyles(t *testing.T) {
	t.Cleanup(resetTheme)
	render := func(value string) string {
		ti := textinput.New()
		ti.Prompt = ": "
		ti.Placeholder = "pods"
		ti.SetWidth(20)
		ti.SetStyles(TextInput())
		ti.SetValue(value)
		ti.Focus()
		return ti.View()
	}
	for _, p := range []palette{monoPalette, colourPalette(true)} {
		use(p)
		if codes := colourCodes(render("po")); len(codes) > 0 {
			t.Errorf("mono=%v: typed input carries colour codes %v", p.mono, codes)
		}
		placeholder := colourCodes(render(""))
		if p.mono && len(placeholder) > 0 {
			t.Errorf("mono placeholder carries colour codes %v", placeholder)
		}
		if !p.mono && len(placeholder) == 0 {
			t.Error("colour placeholder lost its muted colour")
		}
		if !slices.Contains(sgrCodes(render("po")), 7) {
			t.Errorf("mono=%v: cursor is not a reverse-video block", p.mono)
		}
	}
}
