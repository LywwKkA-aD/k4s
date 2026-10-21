package async

import "sync/atomic"

// Token names a view instance (the owner) and one request epoch within it
// (the seq). Every request a view starts carries a Token, and its reply is
// taken only where that Token is still the current one: a reply for another
// view, or for an epoch the view has moved past, is dropped by comparison
// alone. The zero Token belongs to no owner and accepts nothing.
type Token struct {
	owner uint64
	seq   uint64
}

var owners atomic.Uint64

// New returns the first Token of a new owner, distinct from every other
// New in the process. A view calls it once, in its constructor.
func New() Token {
	return Token{owner: owners.Add(1)}
}

// Next returns the owner's next epoch. A view keeps one Token as its mint
// and takes Next for every request epoch it starts, so tokens never repeat
// within the owner.
func (t Token) Next() Token {
	return Token{owner: t.owner, seq: t.seq + 1}
}

// Accept reports whether a reply carrying got belongs to t: the same owner
// and the same epoch.
func (t Token) Accept(got Token) bool {
	return t.owner != 0 && t == got
}
