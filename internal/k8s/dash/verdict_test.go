package dash

import "testing"

func TestAllClearNeedsAWholeFreshPicture(t *testing.T) {
	t.Parallel()

	base := refresh(t, newAPIServer(t, allClearK3s(fixedNow)), Scope{})
	if !base.Verdict.AllClear || base.Verdict.Partial {
		t.Fatalf("fixture is not all clear: %s", base.Verdict)
	}
	cases := []struct {
		name string
		mut  func(*Snapshot)
	}{
		{"truncated events", func(s *Snapshot) { s.Events.Truncated = true }},
		{"nodes stale", func(s *Snapshot) { s.Ledger[Nodes].State = StateStale }},
		{"deployments failed", func(s *Snapshot) { s.Ledger[Deployments].State = StateFailed }},
		{"events absent", func(s *Snapshot) { s.Ledger[Events].State = StateAbsent }},
		{"events forbidden", func(s *Snapshot) { s.Ledger[Events].State = StateForbidden }},
		{"readyz stale", func(s *Snapshot) { s.Ledger[Readyz].State = StateStale }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := base
			tc.mut(&s)
			v := verdictOf(s)
			if v.AllClear || !v.Partial {
				t.Fatalf("verdict = %s (AllClear %v Partial %v)", v, v.AllClear, v.Partial)
			}
		})
	}
}
