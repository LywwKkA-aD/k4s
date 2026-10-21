package status

import (
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

// userHome is $HOME, or "" when it cannot be found.
func userHome() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// redact shortens every home path in s to ~. A match counts only as a whole
// path: what comes before it and after it must not continue a file name, so
// home /home/al leaves /home/alice and /backup/home/al alone.
func redact(s, home string) string {
	home = strings.TrimRight(home, `/\`)
	if home == "" {
		return s
	}
	var b strings.Builder
	pos := 0
	for {
		i := strings.Index(s[pos:], home)
		if i < 0 {
			b.WriteString(s[pos:])
			return b.String()
		}
		start := pos + i
		end := start + len(home)
		b.WriteString(s[pos:start])
		if wholePath(s, start, end) {
			b.WriteString("~")
		} else {
			b.WriteString(home)
		}
		pos = end
	}
}

// wholePath reports whether s[start:end] is neither preceded nor followed by
// a file name character.
func wholePath(s string, start, end int) bool {
	if before, _ := utf8.DecodeLastRuneInString(s[:start]); start > 0 && isNameRune(before) {
		return false
	}
	after, _ := utf8.DecodeRuneInString(s[end:])
	return end == len(s) || !isNameRune(after)
}

func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_'
}
