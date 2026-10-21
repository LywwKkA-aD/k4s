package styles

import (
	"errors"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
		{name: "high contrast", env: map[string]string{"K4S_THEME": "high-contrast"}, want: ModeHighContrast},
		{name: "high contrast in another case", env: map[string]string{"K4S_THEME": " High-Contrast "}, want: ModeHighContrast},
		{name: "no color beats high contrast", env: map[string]string{"NO_COLOR": "1", "K4S_THEME": "high-contrast"}, want: ModeMono},
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
	for _, want := range []string{"K4S_THEME", `"solarized"`, "auto", "dark", "light", "mono", "high-contrast"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %s", err, want)
		}
	}
}

// resetTheme restores the theme the package starts with: dark colours and
// no forced colour profile.
func resetTheme() {
	use(colourPalette(true))
	forcedProfile = nil
}

// The Init tests change this package's styles, so they run sequentially and
// restore the dark colour theme.
func TestInitAppliesTheme(t *testing.T) {
	t.Cleanup(resetTheme)
	cases := []struct {
		name     string
		env      map[string]string
		wantDark bool
		wantMono bool
		wantHint string // Hint's foreground as hex; empty for none
	}{
		{name: "dark", env: map[string]string{"K4S_THEME": "dark"}, wantDark: true, wantHint: tokenMuted.Dark},
		{name: "light", env: map[string]string{"K4S_THEME": "light"}, wantDark: false, wantHint: tokenMuted.Light},
		// Off a terminal nothing can be asked, so high contrast starts dark.
		{name: "high contrast", env: map[string]string{"K4S_THEME": "high-contrast"}, wantDark: true, wantHint: highContrastTokens.muted.Dark},
		{name: "mono via no color", env: map[string]string{"NO_COLOR": "1"}, wantDark: true, wantMono: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Init(envOf(c.env)); err != nil {
				t.Fatalf("Init: %v", err)
			}
			if active.dark != c.wantDark {
				t.Errorf("dark background = %v, want %v", active.dark, c.wantDark)
			}
			_, colourless := Hint.GetForeground().(lipgloss.NoColor)
			if colourless != c.wantMono {
				t.Errorf("Hint colourless = %v, want %v", colourless, c.wantMono)
			}
			if c.wantHint != "" && Hint.GetForeground() != lipgloss.Color(c.wantHint) {
				t.Errorf("Hint fg = %v, want %s", Hint.GetForeground(), c.wantHint)
			}
			if got := Table().Selected.GetReverse(); got != c.wantMono {
				t.Errorf("selected row reverse = %v, want %v", got, c.wantMono)
			}
		})
	}
}

func TestInitAutoDetectsWithoutError(t *testing.T) {
	t.Cleanup(resetTheme)
	if err := Init(envOf(nil)); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// A test binary's stdout is not a terminal, so auto assumes dark.
	if Hint.GetForeground() != tokenMuted.on(true) {
		t.Errorf("auto should use the dark palette off a terminal, Hint fg = %v", Hint.GetForeground())
	}
	if _, forced := ColorProfile(); forced {
		t.Error("auto must leave the colour profile to the program")
	}
}

func TestInitRejectsUnknownThemeAndKeepsStyles(t *testing.T) {
	t.Cleanup(resetTheme)
	use(colourPalette(true))
	err := Init(envOf(map[string]string{"K4S_THEME": "neon", "NO_COLOR": "1"}))
	if !errors.Is(err, ErrInvalidTheme) {
		t.Fatalf("err = %v, want ErrInvalidTheme", err)
	}
	if Hint.GetForeground() != tokenMuted.on(true) {
		t.Error("a rejected theme must not touch the styles")
	}
}
