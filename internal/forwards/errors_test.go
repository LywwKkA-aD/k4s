package forwards

import (
	"errors"
	"testing"
)

func TestFriendlyBindError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		port uint16
		want string
	}{
		{
			name: "privileged port suggests an unprivileged one",
			err:  errors.New("listen tcp 127.0.0.1:80: bind: permission denied"),
			port: 80,
			want: "permission denied binding local port 80 (ports < 1024 need root — try 8080)",
		},
		{
			name: "no suggestion past the last port",
			err:  errors.New("listen tcp 127.0.0.1:60000: bind: permission denied"),
			port: 60000,
			want: "permission denied binding local port 60000 (ports < 1024 need root)",
		},
		{
			name: "port in use",
			err:  errors.New("listen tcp 127.0.0.1:8080: bind: address already in use"),
			port: 8080,
			want: "local port 8080 already in use — pick another",
		},
		{
			name: "unknown error keeps its text",
			err:  errors.New("listen tcp 127.0.0.1:8080: socket: too many open files"),
			port: 8080,
			want: "cannot bind local port 8080: listen tcp 127.0.0.1:8080: socket: too many open files",
		},
		{
			name: "nil error",
			port: 8080,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := FriendlyBindError(tt.err, tt.port); got != tt.want {
				t.Errorf("FriendlyBindError() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCondense(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "known bind failure wins", in: "Unable to listen on port 80\nbind: permission denied\n", want: "bind: permission denied"},
		{name: "first non-empty line", in: "\n  \nerror upgrading connection\nmore\n", want: "error upgrading connection"},
		{name: "blank stays blank", in: "  \n ", want: "  \n "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := condense(tt.in); got != tt.want {
				t.Errorf("condense(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
