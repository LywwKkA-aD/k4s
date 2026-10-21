package async

import "maps"

// InFlight is the set of row actions a view has started and not yet seen
// answered, keyed by action ("logs", "exec", "forward"). The zero value is
// empty. It is a value: every change returns a new set and leaves the
// receiver as it was, so a copy of the view keeps its own.
//
// Use one of Begin or Supersede per action: Begin when a second request
// must be dropped (a second 'e' must not open a second shell), Supersede
// when the newest request wins (a new 'l' on another row).
type InFlight struct {
	reqs map[string]request
}

type request struct {
	tok Token
	key string
}

// Begin starts action under tok unless it is already in flight. ok is
// false, and the set unchanged, for a second request: the first one wins.
func (f InFlight) Begin(action string, tok Token) (InFlight, bool) {
	if f.Busy(action) {
		return f, false
	}
	return f.with(action, request{tok: tok}), true
}

// Supersede starts action for key under tok, replacing a request in flight
// for another key, whose reply is then dropped: the last one wins. A
// repeat for the key already in flight is refused (ok is false), so the
// same row pressed twice resolves once.
func (f InFlight) Supersede(action, key string, tok Token) (InFlight, bool) {
	if r, ok := f.reqs[action]; ok && r.key == key {
		return f, false
	}
	return f.with(action, request{tok: tok, key: key}), true
}

// End finishes action when tok is the request in flight for it. ok is
// false, and the set unchanged, for a stale or foreign reply; the caller
// drops that reply.
func (f InFlight) End(action string, tok Token) (InFlight, bool) {
	if !f.Accept(action, tok) {
		return f, false
	}
	next := maps.Clone(f.reqs)
	delete(next, action)
	return InFlight{reqs: next}, true
}

// Busy reports whether action is in flight.
func (f InFlight) Busy(action string) bool {
	_, ok := f.reqs[action]
	return ok
}

// Accept reports whether tok is the request in flight for action.
func (f InFlight) Accept(action string, tok Token) bool {
	r, ok := f.reqs[action]
	return ok && r.tok.Accept(tok)
}

func (f InFlight) with(action string, r request) InFlight {
	next := make(map[string]request, len(f.reqs)+1)
	maps.Copy(next, f.reqs)
	next[action] = r
	return InFlight{reqs: next}
}
