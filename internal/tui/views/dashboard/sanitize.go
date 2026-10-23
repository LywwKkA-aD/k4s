package dashboard

import (
	"strings"

	"github.com/LywwKkA-aD/k4s/internal/sanitize"
)

// Every string that comes from the cluster (names, statuses, event text,
// the user name, versions) goes through clean before it reaches layout,
// so a crafted object name or event message cannot drive the terminal.

// blanks turns the whitespace a one-line value cannot hold into spaces, as
// the list views do: a tab would widen its cell after it was measured.
var blanks = strings.NewReplacer("\n", " ", "\t", " ")

// clean returns s as one line of inert text (sanitize.Line), with line
// breaks and tabs as spaces.
func clean(s string) string {
	return sanitize.Line(blanks.Replace(s))
}

// cleanArgs cleans every argument of a command into a new slice.
func cleanArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = clean(a)
	}
	return out
}
