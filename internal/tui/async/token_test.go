package async_test

import (
	"sync"
	"testing"

	"github.com/LywwKkA-aD/k4s/internal/tui/async"
)

func TestNewTokensAreDistinct(t *testing.T) {
	t.Parallel()
	const n = 64
	toks := make(chan async.Token, n)
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() { toks <- async.New() })
	}
	wg.Wait()
	close(toks)

	seen := make(map[async.Token]bool, n)
	for tok := range toks {
		if seen[tok] {
			t.Fatalf("New returned %v twice", tok)
		}
		seen[tok] = true
	}
}

func TestTokenAccept(t *testing.T) {
	t.Parallel()
	a, b := async.New(), async.New()

	tests := []struct {
		name     string
		current  async.Token
		reply    async.Token
		accepted bool
	}{
		{name: "same token", current: a, reply: a, accepted: true},
		{name: "same epoch reached twice", current: a.Next().Next(), reply: a.Next().Next(), accepted: true},
		{name: "reply from an earlier epoch", current: a.Next(), reply: a, accepted: false},
		{name: "reply from a later epoch", current: a, reply: a.Next(), accepted: false},
		{name: "reply for another owner", current: a, reply: b, accepted: false},
		{name: "same epoch of another owner", current: a.Next(), reply: b.Next(), accepted: false},
		{name: "zero reply", current: a, reply: async.Token{}, accepted: false},
		{name: "zero current accepts nothing", current: async.Token{}, reply: async.Token{}, accepted: false},
		{name: "next of zero accepts nothing", current: async.Token{}.Next(), reply: async.Token{}.Next(), accepted: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.current.Accept(tc.reply); got != tc.accepted {
				t.Fatalf("Accept = %v, want %v", got, tc.accepted)
			}
		})
	}
}

func TestTokenNextLeavesReceiver(t *testing.T) {
	t.Parallel()
	a := async.New()
	before := a
	_ = a.Next()
	if a != before {
		t.Fatal("Next changed its receiver")
	}
}
