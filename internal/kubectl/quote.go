package kubectl

import "strings"

// Quote returns s as one POSIX shell word. Words made only of letters,
// digits and @%+=:,./_- stay bare; anything else, the empty word included,
// is single-quoted. An embedded single quote closes the quoting, is written
// as \' and reopens it.
func Quote(s string) string {
	if s != "" && isBare(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isBare reports whether s survives sh, bash and zsh unquoted. A leading =
// is quoted because zsh expands =cmd to the path of cmd.
func isBare(s string) bool {
	if s[0] == '=' {
		return false
	}
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case 'a' <= b && b <= 'z', 'A' <= b && b <= 'Z', '0' <= b && b <= '9':
		case strings.IndexByte("@%+=:,./_-", b) >= 0:
		default:
			return false
		}
	}
	return true
}
