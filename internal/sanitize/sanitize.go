// Package sanitize turns text from the cluster (log lines, events, describe
// and YAML output, object names) into inert text before k4s draws it. A
// workload controls what it writes to its logs, so without this a pod could
// set the operator's clipboard with OSC 52, retitle or clear the terminal,
// or move the cursor over k4s's own frame.
//
// The package has no Charm imports, so k8s-side code can use it too.
package sanitize

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxLine is the most bytes of one line k4s keeps; the rest is replaced by
// a "…[+N bytes]" note.
const MaxLine = 64 << 10

const (
	esc = 0x1b
	bel = 0x07

	// C1 controls that open a sequence when they arrive as runes.
	c1DCS = 0x90
	c1SOS = 0x98
	c1CSI = 0x9b
	c1ST  = 0x9c
	c1OSC = 0x9d
	c1PM  = 0x9e
	c1APC = 0x9f
)

// Line returns s as one line of inert text:
//   - escape sequences (CSI, OSC, DCS, SOS, PM, APC and the short ESC
//     sequences) are removed together with their payload, so application
//     colours go too;
//   - every other C0 and C1 control is dropped, tab excepted, and so are
//     CR, LF and the invisible format characters (bidi controls,
//     zero-width characters, tags; see invisible);
//   - invalid UTF-8 becomes U+FFFD;
//   - text over MaxLine bytes is cut there and noted with "…[+N bytes]".
func Line(s string) string {
	if clean(s, false) {
		return s
	}
	return capped(scrub(s, false))
}

// Text is Line for text of several lines: it keeps LF, drops CR (so CRLF
// becomes LF) and caps each line on its own. An escape sequence never
// runs past the end of its line.
func Text(s string) string {
	if !strings.ContainsRune(s, '\n') {
		return Line(s)
	}
	if clean(s, true) {
		return s
	}
	lines := strings.Split(scrub(s, true), "\n")
	for i, l := range lines {
		lines[i] = capped(l)
	}
	return strings.Join(lines, "\n")
}

// clean reports whether s is printable ASCII and tabs, and line feeds when
// keepLF, within the cap: the common case for log lines, which then need
// no copy. No line of s can pass the cap when s does not.
func clean(s string, keepLF bool) bool {
	if len(s) > MaxLine {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\n' && keepLF {
			continue
		}
		if (c < 0x20 && c != '\t') || c >= 0x7f {
			return false
		}
	}
	return true
}

func capped(s string) string {
	if len(s) <= MaxLine {
		return s
	}
	cut := MaxLine
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…[+" + strconv.Itoa(len(s)-cut) + " bytes]"
}

// scrub removes controls and escape sequences. keepLF keeps line feeds and
// ends any open sequence at one.
func scrub(s string, keepLF bool) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if next, ok := skipSequence(s, i, r, size, keepLF); ok {
			i = next
			continue
		}
		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError)
		case r == '\n' && keepLF, printable(r):
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// skipSequence returns the index after the escape sequence that the rune r
// at i opens, or false when r opens none.
func skipSequence(s string, i int, r rune, size int, keepLF bool) (int, bool) {
	switch r {
	case esc:
		return skipEscape(s, i+1, keepLF), true
	case c1CSI:
		return skipCSI(s, i+size, keepLF), true
	case c1OSC, c1DCS, c1SOS, c1PM, c1APC:
		return skipString(s, i+size, keepLF), true
	}
	return i, false
}

// printable reports whether r is text: tab, or anything but a C0 or C1
// control, DEL and the invisible format characters.
func printable(r rune) bool {
	return r == '\t' || (r >= 0x20 && r != 0x7f && (r < 0x80 || r > 0x9f) && !invisible(r))
}

// invisible reports the format characters a terminal draws as nothing or
// as a reordering of the text around them:
//   - the bidi marks, embeddings, overrides and isolates (U+061C,
//     U+200E-U+200F, U+202A-U+202E, U+2066-U+2069): a name carrying one
//     shows its text in another order than it is read, so a pasted
//     command would not be the one seen;
//   - the zero-width characters (U+200B-U+200D, U+2060-U+2064, U+FEFF),
//     the soft hyphen (U+00AD), the line and paragraph separators
//     (U+2028-U+2029), the deprecated format controls (U+206A-U+206F),
//     the interlinear annotations (U+FFF9-U+FFFB) and the tags
//     (U+E0000-U+E007F): they make two names that read alike differ, or
//     hide text in one.
func invisible(r rune) bool {
	switch {
	case r < 0xad:
		return false
	case r == 0xad, r == 0x61c, r == 0xfeff:
		return true
	}
	return (r >= 0x200b && r <= 0x200f) || (r >= 0x2028 && r <= 0x202e) ||
		(r >= 0x2060 && r <= 0x2064) || (r >= 0x2066 && r <= 0x206f) ||
		(r >= 0xfff9 && r <= 0xfffb) || (r >= 0xe0000 && r <= 0xe007f)
}

// skipEscape returns the index after the sequence whose ESC ended at i.
func skipEscape(s string, i int, keepLF bool) int {
	if i >= len(s) {
		return i
	}
	switch s[i] {
	case '[':
		return skipCSI(s, i+1, keepLF)
	case ']', 'P', 'X', '^', '_':
		return skipString(s, i+1, keepLF)
	}
	// nF sequences (charset designation and the like) carry intermediate
	// bytes before their final byte; the rest are ESC plus one byte.
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
		i++
	}
	if i < len(s) && stops(s[i], keepLF) {
		return i
	}
	return min(i+1, len(s))
}

// skipCSI returns the index after the final byte of a control sequence
// whose introducer ended at i.
func skipCSI(s string, i int, keepLF bool) int {
	for ; i < len(s); i++ {
		c := s[i]
		if stops(c, keepLF) {
			return i
		}
		if c >= 0x40 && c <= 0x7e {
			return i + 1
		}
	}
	return i
}

// skipString returns the index after the terminator (BEL, ESC \ or the C1
// ST) of a string sequence whose introducer ended at i.
func skipString(s string, i int, keepLF bool) int {
	for i < len(s) {
		c := s[i]
		switch {
		case stops(c, keepLF):
			return i
		case c == bel:
			return i + 1
		case c == esc && i+1 < len(s) && s[i+1] == '\\':
			return i + 2
		case strings.HasPrefix(s[i:], string(rune(c1ST))):
			return i + len(string(rune(c1ST)))
		}
		i++
	}
	return i
}

func stops(c byte, keepLF bool) bool {
	return keepLF && c == '\n'
}
