//go:build !unix

package logsink

// noFollow is zero where the platform has no O_NOFOLLOW, Windows among
// them; the private directory openLogFile makes is the only guard there.
const noFollow = 0
