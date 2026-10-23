package dash

// maxProblemRows caps PodsPanel.Problems. A cluster can list any number
// of failing pods, and every frame walks the rows; this is more than a
// zoomed panel 3 shows on any terminal and than next needs to find its
// five. Past it the least severe and least recent rows go, and Cut keeps
// what they would have counted. Cut rows are not flagged for the warning
// rule, so a warning about one of them may count.
const maxProblemRows = 500

// ProblemCounts counts problem rows the way the panels and chips do.
type ProblemCounts struct {
	Rows      int // every row
	Attention int // ▲ and △ rows
	Waiting   int // △ rows of pods past their start grace: "N not ready"
}

// add returns c with r counted.
func (c ProblemCounts) add(r PodRow) ProblemCounts {
	c.Rows++
	if r.Sev >= SevWaiting {
		c.Attention++
	}
	if r.Sev == SevWaiting && !r.Starting {
		c.Waiting++
	}
	return c
}

// Attention counts the ▲ and △ problem rows, those past the cap included.
func (p PodsPanel) Attention() int { return p.counts().Attention }

// Waiting counts the △ rows of pods past their start grace, those past the
// cap included.
func (p PodsPanel) Waiting() int { return p.counts().Waiting }

// counts is Cut with the rows Problems kept.
func (p PodsPanel) counts() ProblemCounts {
	c := p.Cut
	for _, r := range p.Problems {
		c = c.add(r)
	}
	return c
}
