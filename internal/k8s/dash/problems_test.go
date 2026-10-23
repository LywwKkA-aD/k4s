package dash

import (
	"fmt"
	"slices"
	"testing"
	"time"
)

// TestProblemsAreCappedAndCountsStayTrue: a hostile or very broken cluster
// can list any number of failing pods. Problems keeps the most severe
// maxProblemRows of them; every count drawn from them stays true.
func TestProblemsAreCappedAndCountsStayTrue(t *testing.T) {
	t.Parallel()

	const crashing, waiting = maxProblemRows + 40, 30
	pods := make([]podRow, 0, crashing+waiting)
	for i := range waiting {
		pods = append(pods, podRow{name: fmt.Sprintf("w-%d", i), namespace: "shop", status: "Running",
			readyOf: 1, readyOK: true, created: fixedNow.Add(-time.Hour)})
	}
	for i := range crashing {
		pods = append(pods, podRow{name: fmt.Sprintf("c-%d", i), namespace: "shop", status: "CrashLoopBackOff",
			readyOf: 1, readyOK: true, restarts: 3, created: fixedNow.Add(-time.Hour)})
	}
	p := derivePods(store{pods: pods}, Ledger{}, Scope{}, fixedNow)

	if len(p.Problems) != maxProblemRows {
		t.Fatalf("kept %d problem rows, want %d", len(p.Problems), maxProblemRows)
	}
	if got, want := p.Cut.Rows, crashing+waiting-maxProblemRows; got != want {
		t.Errorf("cut rows = %d, want %d", got, want)
	}
	if last := p.Problems[len(p.Problems)-1]; last.Sev != SevFailing {
		t.Errorf("the cap kept a %v row before the failing ones", last.Sev)
	}
	if got := p.Attention(); got != crashing+waiting {
		t.Errorf("Attention() = %d, want %d", got, crashing+waiting)
	}
	if got := p.Waiting(); got != waiting {
		t.Errorf("Waiting() = %d, want %d", got, waiting)
	}
	if got, want := p.Failing, []ReasonCount{{Reason: "CrashLoopBackOff", Count: crashing}}; !slices.Equal(got, want) {
		t.Errorf("Failing = %v, want %v", got, want)
	}
	chips := podChips(p)
	if len(chips) != 2 || chips[1].Text != fmt.Sprintf("%d not ready", waiting) {
		t.Errorf("chips = %+v, want the true not-ready count", chips)
	}
}

func TestProblemCountsAddEveryRow(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		p    PodsPanel
		want ProblemCounts
	}{
		{name: "none", want: ProblemCounts{}},
		{
			name: "kept rows",
			p: PodsPanel{Problems: []PodRow{
				{Sev: SevFailing}, {Sev: SevWaiting}, {Sev: SevWaiting, Starting: true}, {Sev: SevMinor},
			}},
			want: ProblemCounts{Rows: 4, Attention: 3, Waiting: 1},
		},
		{
			name: "kept and cut rows",
			p:    PodsPanel{Problems: []PodRow{{Sev: SevWaiting}}, Cut: ProblemCounts{Rows: 7, Attention: 5, Waiting: 2}},
			want: ProblemCounts{Rows: 8, Attention: 6, Waiting: 3},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.counts(); got != tc.want {
				t.Fatalf("counts() = %+v, want %+v", got, tc.want)
			}
			if tc.p.Attention() != tc.want.Attention || tc.p.Waiting() != tc.want.Waiting {
				t.Fatalf("Attention() = %d, Waiting() = %d, want %+v", tc.p.Attention(), tc.p.Waiting(), tc.want)
			}
		})
	}
}
