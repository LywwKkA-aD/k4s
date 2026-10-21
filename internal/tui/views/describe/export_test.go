package describe

import "time"

// WithBuiltInTimeout bounds the built-in describer by d instead of
// fetchTimeout, so a test of a hung API need not wait ten seconds.
func WithBuiltInTimeout(d time.Duration) Option {
	return func(m Model) Model {
		m.builtInTimeout = d
		return m
	}
}
