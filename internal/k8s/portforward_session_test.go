package k8s

import (
	"errors"
	"strings"
	"testing"
	"time"

	streamhttp "k8s.io/streaming/pkg/httpstream"
)

type panickingDialer struct{}

func (panickingDialer) Dial(_ ...string) (streamhttp.Connection, string, error) {
	panic("dialer bug")
}

// A panic on the forwarder's goroutine ends the session with ErrInternal
// instead of the process.
func TestAForwarderPanicEndsTheSessionNotTheProcess(t *testing.T) {
	t.Parallel()
	released := make(chan struct{})
	sess, err := runForwarder(panickingDialer{}, "web", freeLocalPort(t), 80, make(chan struct{}),
		func() { close(released) }, time.Now)
	if err != nil {
		t.Fatalf("runForwarder: %v", err)
	}
	select {
	case err := <-sess.Err:
		if !errors.Is(err, ErrInternal) || !strings.Contains(err.Error(), "dialer bug") {
			t.Errorf("Err = %v, want ErrInternal with the panic", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no error after the panic")
	}
	select {
	case <-sess.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("the session never finished")
	}
	select {
	case <-released:
	default:
		t.Error("the startup context was not released")
	}
}
