package styles

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func envOf(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func TestParseMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		env     map[string]string
		want    Mode
		wantErr bool
	}{
		{name: "unset defaults to auto", env: nil, want: ModeAuto},
		{name: "explicit auto", env: map[string]string{"K4S_THEME": "auto"}, want: ModeAuto},
		{name: "dark", env: map[string]string{"K4S_THEME": "dark"}, want: ModeDark},
		{name: "light", env: map[string]string{"K4S_THEME": "light"}, want: ModeLight},
		{name: "mono", env: map[string]string{"K4S_THEME": "mono"}, want: ModeMono},
		{name: "case and spaces ignored", env: map[string]string{"K4S_THEME": " Light "}, want: ModeLight},
		{name: "no color forces mono", env: map[string]string{"NO_COLOR": "1"}, want: ModeMono},
		{name: "no color beats a colour theme", env: map[string]string{"NO_COLOR": "1", "K4S_THEME": "dark"}, want: ModeMono},
		{name: "empty no color is ignored", env: map[string]string{"NO_COLOR": ""}, want: ModeAuto},
		{name: "unknown theme", env: map[string]string{"K4S_THEME": "solarized"}, wantErr: true},
		{name: "unknown theme even with no color", env: map[string]string{"K4S_THEME": "solarized", "NO_COLOR": "1"}, wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMode(envOf(c.env))
			if c.wantErr {
				if !errors.Is(err, ErrInvalidTheme) {
					t.Fatalf("err = %v, want ErrInvalidTheme", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("mode = %q, want %q", got, c.want)
			}
		})
	}
}

func TestParseModeErrorNamesValueAndChoices(t *testing.T) {
	t.Parallel()
	_, err := ParseMode(envOf(map[string]string{"K4S_THEME": "solarized"}))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"K4S_THEME", `"solarized"`, "auto", "dark", "light", "mono"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %s", err, want)
		}
	}
}

// The Init tests change lipgloss's global renderer and this package's
// styles, so they run sequentially and restore the colour theme.
func TestInitAppliesTheme(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	cases := []struct {
		name     string
		env      map[string]string
		wantDark bool
		wantMono bool
	}{
		{name: "dark", env: map[string]string{"K4S_THEME": "dark"}, wantDark: true},
		{name: "light", env: map[string]string{"K4S_THEME": "light"}, wantDark: false},
		{name: "mono via no color", env: map[string]string{"NO_COLOR": "1"}, wantDark: true, wantMono: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Init(envOf(c.env)); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if got := lipgloss.HasDarkBackground(); got != c.wantDark {
				t.Errorf("HasDarkBackground = %v, want %v", got, c.wantDark)
			}
			_, colourless := Hint.GetForeground().(lipgloss.NoColor)
			if colourless != c.wantMono {
				t.Errorf("Hint colourless = %v, want %v", colourless, c.wantMono)
			}
			if got := Table().Selected.GetReverse(); got != c.wantMono {
				t.Errorf("selected row reverse = %v, want %v", got, c.wantMono)
			}
		})
	}
}

func TestInitAutoDetectsWithoutError(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	if err := Init(envOf(nil)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if Hint.GetForeground() != tokenMuted {
		t.Errorf("auto should use the adaptive palette, Hint fg = %v", Hint.GetForeground())
	}
}

func TestInitRejectsUnknownThemeAndKeepsStyles(t *testing.T) {
	t.Cleanup(func() { use(colourPalette) })
	use(colourPalette)
	err := Init(envOf(map[string]string{"K4S_THEME": "neon", "NO_COLOR": "1"}))
	if !errors.Is(err, ErrInvalidTheme) {
		t.Fatalf("err = %v, want ErrInvalidTheme", err)
	}
	if Hint.GetForeground() != tokenMuted {
		t.Error("a rejected theme must not touch the styles")
	}
}
