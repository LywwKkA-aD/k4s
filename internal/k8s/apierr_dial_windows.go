package k8s

import "syscall"

// unreachableErrnos are what a dial fails with when nothing listens at the
// address or no route leads there. Winsock reports its own codes, which
// the syscall package does not name; the POSIX names stay for errors built
// from them.
var unreachableErrnos = []error{
	syscall.ECONNREFUSED, syscall.EHOSTUNREACH, syscall.ENETUNREACH,
	syscall.Errno(10061), // WSAECONNREFUSED
	syscall.Errno(10065), // WSAEHOSTUNREACH
	syscall.Errno(10051), // WSAENETUNREACH
}
