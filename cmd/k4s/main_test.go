package main

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"
)

func TestRunError(t *testing.T) {
	t.Parallel()

	noTTY := &fs.PathError{Op: "open", Path: "/dev/tty", Err: syscall.ENXIO}
	cases := []struct {
		name string
		in   error
		want string
	}{
		{
			name: "no terminal collapses the framework's double prefix",
			in:   fmt.Errorf("bubbletea: error opening TTY: %w", fmt.Errorf("bubbletea: could not open TTY: %w", noTTY)),
			want: "no terminal: open /dev/tty: " + syscall.ENXIO.Error(),
		},
		{name: "other errors pass through", in: errors.New("program was killed"), want: "program was killed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := runError(tc.in)
			if got.Error() != tc.want {
				t.Fatalf("runError = %q, want %q", got, tc.want)
			}
			if strings.Count(got.Error(), "bubbletea") != 0 {
				t.Fatalf("framework prefix leaked: %q", got)
			}
		})
	}
	if runError(nil) != nil {
		t.Fatal("runError(nil) must be nil")
	}
}
