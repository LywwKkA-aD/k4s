package logsink

import (
	"path/filepath"
	"testing"
)

func TestResolvePath(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/home/ada")
	fallback := filepath.Join(home, ".local", "state", "k4s", "k4s.log")

	cases := []struct {
		name      string
		stateHome string
		home      string
		want      string
	}{
		{name: "absolute xdg wins", stateHome: filepath.FromSlash("/var/state"), home: home, want: filepath.Join(filepath.FromSlash("/var/state"), "k4s", "k4s.log")},
		{name: "relative xdg ignored", stateHome: "state", home: home, want: fallback},
		{name: "empty xdg uses home", stateHome: "", home: home, want: fallback},
		{name: "no home and no xdg", stateHome: "", home: "", want: ""},
		{name: "relative home rejected", stateHome: "", home: "ada", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolvePath(tc.stateHome, tc.home); got != tc.want {
				t.Fatalf("ResolvePath(%q, %q) = %q, want %q", tc.stateHome, tc.home, got, tc.want)
			}
		})
	}
}

func TestDisplayPath(t *testing.T) {
	t.Parallel()

	home := filepath.FromSlash("/home/ada")
	cases := []struct {
		name string
		path string
		home string
		want string
	}{
		{name: "under home", path: filepath.Join(home, ".local", "state", "k4s", "k4s.log"), home: home, want: "~" + string(filepath.Separator) + filepath.Join(".local", "state", "k4s", "k4s.log")},
		{name: "outside home", path: filepath.FromSlash("/var/state/k4s/k4s.log"), home: home, want: filepath.FromSlash("/var/state/k4s/k4s.log")},
		{name: "sibling with shared prefix", path: filepath.FromSlash("/home/adam/k4s.log"), home: home, want: filepath.FromSlash("/home/adam/k4s.log")},
		{name: "no home", path: filepath.FromSlash("/home/ada/k4s.log"), home: "", want: filepath.FromSlash("/home/ada/k4s.log")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := displayPath(tc.path, tc.home); got != tc.want {
				t.Fatalf("displayPath(%q, %q) = %q, want %q", tc.path, tc.home, got, tc.want)
			}
		})
	}
}
