package dashboard

import (
	"math"
	"time"
)

// minuteLimit is 60 minutes of one-minute cells, the span of the rings
// they are drawn beside.
const minuteLimit = historyLimit / int(time.Minute/metricsStep)

// clockMinutes is one measure a wall-clock minute a cell, for the spark rows:
// the means of the minutes that closed, oldest first, and the minute
// still open. Binning on the clock keeps a closed minute fixed, so a new
// sample redraws only the open cell. It is a value; add returns a new one.
type clockMinutes struct {
	closed series
	open   time.Time // the open minute, zero before the first sample
	sum    float64
	n      int // samples in the open minute that were known
}

// add folds in v, sampled at wall time at. A sample in a later minute
// closes the open one, and every minute between them is blank.
func (m clockMinutes) add(at time.Time, v float64) clockMinutes {
	minute := at.Truncate(time.Minute)
	switch {
	case m.open.IsZero():
		m.open = minute
	case minute.After(m.open):
		skipped := int(minute.Sub(m.open)/time.Minute) - 1
		m.closed = m.closed.push(m.mean(), minuteLimit).gap(skipped, minuteLimit)
		m.open, m.sum, m.n = minute, 0, 0
	}
	if !math.IsNaN(v) {
		m.sum, m.n = m.sum+v, m.n+1
	}
	return m
}

// mean is the open minute's mean, NaN when none of its samples was known.
func (m clockMinutes) mean() float64 {
	if m.n == 0 {
		return math.NaN()
	}
	return m.sum / float64(m.n)
}

// values returns the cells, oldest first, the open minute last, at most
// minuteLimit of them. The slice is the caller's own.
func (m clockMinutes) values() []float64 {
	if m.open.IsZero() {
		return nil
	}
	closed := m.closed.values()
	closed = closed[max(len(closed)+1-minuteLimit, 0):]
	out := make([]float64, 0, len(closed)+1)
	out = append(out, closed...)
	return append(out, m.mean())
}
