package logsink

import (
	"path/filepath"
	"strings"
)

// ResolvePath returns where k4s keeps its log: $XDG_STATE_HOME/k4s/k4s.log
// when stateHome is absolute (the XDG spec says relative values are
// invalid and must be ignored), else ~/.local/state/k4s/k4s.log. It returns
// "" when neither yields an absolute path.
func ResolvePath(stateHome, home string) string {
	if filepath.IsAbs(stateHome) {
		return filepath.Join(stateHome, "k4s", "k4s.log")
	}
	if filepath.IsAbs(home) {
		return filepath.Join(home, ".local", "state", "k4s", "k4s.log")
	}
	return ""
}

// displayPath shortens a path under home to ~/... for messages.
func displayPath(path, home string) string {
	if home == "" {
		return path
	}
	rel, ok := strings.CutPrefix(path, filepath.Clean(home)+string(filepath.Separator))
	if !ok {
		return path
	}
	return "~" + string(filepath.Separator) + rel
}
