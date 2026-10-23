package dash

import "testing"

// TestWarningsChipOfACappedPageIsAtLeast: the warnings counted on a page
// cut at the cap are a lower bound, and the chip says so.
func TestWarningsChipOfACappedPageIsAtLeast(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		truncated bool
		want      string
	}{
		{name: "a whole list", want: "3 warnings in 10m"},
		{name: "a capped page", truncated: true, want: "3+ warnings in 10m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var s Snapshot
			s.Events.Counted, s.Events.Truncated = 3, tc.truncated
			v := verdictOf(s)
			if len(v.Chips) != 1 || v.Chips[0].Text != tc.want {
				t.Fatalf("chips = %s, want %q", v, tc.want)
			}
		})
	}
}

// TestServiceCountIsAtLeastWithoutARemainingCount: the service count is a
// limit=1 list. With a remaining count it is exact; a server that cuts the
// list without one says only that there is more, so the count is a lower
// bound and the panel reads it as N+ (M4.3).
func TestServiceCountIsAtLeastWithoutARemainingCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		opts     []option
		services int
		approx   bool
	}{
		{name: "a remaining count", services: 4},
		{name: "a continue token only", opts: []option{withContinueOnly()}, services: 1, approx: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			n := refresh(t, newAPIServer(t, busyK3s(fixedNow), tc.opts...), Scope{}).Cluster.Network
			if !n.ServicesKnown || n.Services != tc.services || n.ServicesApprox != tc.approx {
				t.Fatalf("network = %d services (known %v, approx %v), want %d (approx %v)",
					n.Services, n.ServicesKnown, n.ServicesApprox, tc.services, tc.approx)
			}
		})
	}
}
