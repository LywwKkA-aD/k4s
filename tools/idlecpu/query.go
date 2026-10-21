package main

import (
	"bytes"
	"slices"
)

// Replies a real terminal would send. Without them k4s waits out its
// background-colour query at start-up, which a person's terminal never
// makes it do.
const (
	replyDA1           = "\x1b[?62;22c"
	replyBackgroundBEL = "\x1b]11;rgb:1e1e/1e1e/1e1e\x07"
	replyBackgroundST  = "\x1b]11;rgb:1e1e/1e1e/1e1e\x1b\\"
	replyCPR           = "\x1b[1;1R"
)

// termQuery is a request the TUI writes to its terminal and the reply.
type termQuery struct {
	seq, reply string
}

var terminalQueries = []termQuery{
	{seq: "\x1b[c", reply: replyDA1},
	{seq: "\x1b[0c", reply: replyDA1},
	{seq: "\x1b]11;?\x07", reply: replyBackgroundBEL},
	{seq: "\x1b]11;?\x1b\\", reply: replyBackgroundST},
	{seq: "\x1b[6n", reply: replyCPR},
}

// responder finds terminal queries in the output stream. A query can be
// split across reads, so the tail of each read is carried into the next.
type responder struct {
	queries []termQuery
	longest int
	carry   []byte
}

func newResponder(queries []termQuery) responder {
	longest := 0
	for _, q := range queries {
		longest = max(longest, len(q.seq))
	}
	return responder{queries: queries, longest: longest}
}

// scan returns the responder for the next read and the replies to every
// query that ends inside chunk.
func (r responder) scan(chunk []byte) (responder, []byte) {
	buf := append(slices.Clone(r.carry), chunk...)
	var replies []byte
	for i, b := range buf {
		if b != 0x1b {
			continue
		}
		// A query that ends inside the carry was answered on the last read.
		if q, ok := r.match(buf[i:]); ok && i+len(q.seq) > len(r.carry) {
			replies = append(replies, q.reply...)
		}
	}
	keep := min(len(buf), r.longest-1)
	return responder{queries: r.queries, longest: r.longest, carry: slices.Clone(buf[len(buf)-keep:])}, replies
}

func (r responder) match(b []byte) (termQuery, bool) {
	for _, q := range r.queries {
		if bytes.HasPrefix(b, []byte(q.seq)) {
			return q, true
		}
	}
	return termQuery{}, false
}
