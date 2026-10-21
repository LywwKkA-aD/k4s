// Package dash is the data layer of the k4s dashboard: what to poll, how
// to poll it cheaply, and what the panels show, as plain data.
//
// One refresh is three steps, each a value in and a value out:
//
//	in   := snap.PlanInput(elapsed, watch, force, focus)
//	plan := MakePlan(in)                    // pure: due set and next interval
//	snap  = fetcher.Fetch(ctx, plan, snap)  // requests, then a new Snapshot
//
// MakePlan reads no clock and starts nothing. Fetch runs the due requests
// (at most eight at once), then a few typed lookups that give flagged
// Table rows their namespace and crashing container, and derives every
// panel. Denied, absent and stale sources are states in the panels, never
// errors. Every panel and row carries the exact read-only kubectl command
// it stands for, with Approx set where k4s summarises.
//
// The package must not import Charm or internal/tui.
package dash
