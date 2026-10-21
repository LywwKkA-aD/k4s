//go:build !windows

package k8s

import "syscall"

// unreachableErrnos are what a dial fails with when nothing listens at the
// address or no route leads there.
var unreachableErrnos = []error{syscall.ECONNREFUSED, syscall.EHOSTUNREACH, syscall.ENETUNREACH}
