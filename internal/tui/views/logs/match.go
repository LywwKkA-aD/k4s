package logs

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

// span is one occurrence of the search query in a line's text, as byte
// offsets.
type span struct {
	start, end int
}

// match is one occurrence in the backlog: the line it is on and where.
type match struct {
	line int
	span
}

// query is a search as typed, smart-case: without an upper-case letter it
// ignores case, with one it matches exactly, as in less and vim.
type query struct {
	text string
	fold bool
}

func newQuery(text string) query {
	return query{text: text, fold: !strings.ContainsFunc(text, unicode.IsUpper)}
}

// find returns every occurrence of q in s, left to right, never
// overlapping: what the counter counts and the highlight paints.
func (q query) find(s string) []span {
	if q.text == "" {
		return nil
	}
	var out []span
	for at := 0; at < len(s); {
		start, end := q.index(s[at:])
		if start < 0 {
			break
		}
		out = append(out, span{start: at + start, end: at + end})
		at += end
	}
	return out
}

// index returns the first occurrence of q in s, or -1.
func (q query) index(s string) (int, int) {
	if !q.fold {
		i := strings.Index(s, q.text)
		if i < 0 {
			return -1, -1
		}
		return i, i + len(q.text)
	}
	for i := 0; i < len(s); {
		if n := foldPrefix(s[i:], q.text); n > 0 {
			return i, i + n
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return -1, -1
}

// foldPrefix returns how many bytes of s equal q under Unicode simple case
// folding, or 0 when s does not start with q. The lengths may differ: the
// Kelvin sign folds to k.
func foldPrefix(s, q string) int {
	i := 0
	for _, want := range q {
		if i >= len(s) {
			return 0
		}
		got, size := utf8.DecodeRuneInString(s[i:])
		if (got == utf8.RuneError && size == 1) || !equalFold(got, want) {
			return 0
		}
		i += size
	}
	return i
}

func equalFold(a, b rune) bool {
	if a == b {
		return true
	}
	for r := unicode.SimpleFold(a); r != a; r = unicode.SimpleFold(r) {
		if r == b {
			return true
		}
	}
	return false
}

// rowWithin is the row, counted from 0, that the byte at offset of plain
// lands on once plain is soft-wrapped to width as the view wraps it.
// Wrapping drops the spaces it breaks at, so each row is found again in
// plain after skipping them; an offset on a dropped space belongs to the
// next row.
func rowWithin(plain string, offset, width int) int {
	if width <= 0 {
		return 0
	}
	rows := strings.Split(ansi.Wrap(plain, width, ""), "\n")
	pos := 0
	for r, row := range rows {
		for pos < len(plain) && !strings.HasPrefix(plain[pos:], row) {
			pos++
		}
		pos += len(row)
		if offset < pos {
			return r
		}
	}
	return len(rows) - 1
}
