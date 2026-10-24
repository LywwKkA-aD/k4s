//go:build unix

package logsink

import "syscall"

// noFollow makes the open fail on a symbolic link at the log path, so a
// link planted there cannot make k4s append to, or create, its target.
const noFollow = syscall.O_NOFOLLOW
