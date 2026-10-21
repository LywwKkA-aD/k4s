package k8s

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// panickingStream is a stream with a bug in every method it is read,
// written or closed through.
type panickingStream struct{ streamhttp.Stream }

func (panickingStream) Read([]byte) (int, error)  { panic("read bug") }
func (panickingStream) Write([]byte) (int, error) { panic("write bug") }
func (panickingStream) Close() error              { panic("close bug") }

// panickingConn is a connection whose error streams panic when read, or
// whose CreateStream panics outright.
type panickingConn struct {
	*fakeConn
	onCreate bool
}

func (c panickingConn) CreateStream(h http.Header) (streamhttp.Stream, error) {
	if c.onCreate {
		panic("create bug")
	}
	s, err := c.fakeConn.CreateStream(h)
	if err != nil || h.Get(corev1.StreamType) != corev1.StreamTypeError {
		return s, err
	}
	return panickingStream{Stream: s}, nil
}

type panickingConnDialer struct{ conn panickingConn }

func (d panickingConnDialer) Dial(_ ...string) (streamhttp.Connection, string, error) {
	return d.conn, "portforward.k8s.io", nil
}

// requireInternal checks that err is ErrInternal carrying bug, and that the
// session recorded it as its latest tunnel error.
func requireInternal(t *testing.T, err error, errs *tunnelErrors, bug string) {
	t.Helper()
	if !errors.Is(err, ErrInternal) || !strings.Contains(err.Error(), bug) {
		t.Errorf("err = %v, want ErrInternal with %q", err, bug)
	}
	if got := errs.last().Message; !strings.Contains(got, ErrInternal.Error()) || !strings.Contains(got, bug) {
		t.Errorf("LastError = %q, want the internal error with %q", got, bug)
	}
	select {
	case <-errs.changed:
	default:
		t.Error("the session did not say a tunnel error arrived")
	}
}

// TestAPanickingWrappedStreamIsAnInternalError: the wrappers run on
// client-go's per-connection goroutines, which no recover of k4s covers,
// so each catches a panic of its own or of the stream it wraps.
func TestAPanickingWrappedStreamIsAnInternalError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		bug  string
		call func(s *observedStream) error
	}{
		{name: "read", bug: "read bug", call: func(s *observedStream) error { _, err := s.Read(make([]byte, 8)); return err }},
		{name: "write", bug: "write bug", call: func(s *observedStream) error { _, err := s.Write([]byte("x")); return err }},
		{name: "close", bug: "close bug", call: func(s *observedStream) error { return s.Close() }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			errs := newTunnelErrors(time.Now)
			s := &observedStream{Stream: panickingStream{}, errs: errs}
			requireInternal(t, tc.call(s), errs, tc.bug)
		})
	}
}

func TestAPanickingCreateStreamIsAnInternalError(t *testing.T) {
	t.Parallel()

	errs := newTunnelErrors(time.Now)
	conn, _, err := observe(panickingConnDialer{conn: panickingConn{fakeConn: newFakeConn(), onCreate: true}}, errs).Dial()
	if err != nil {
		t.Fatal(err)
	}
	s, err := conn.CreateStream(http.Header{})
	if s != nil {
		t.Errorf("CreateStream returned a stream after a panic: %v", s)
	}
	requireInternal(t, err, errs, "create bug")
}

// TestAPanicOnAConnectionKeepsTheSession runs client-go's forwarder over a
// connection whose error stream panics: the session records it, and the
// process and the forward live on.
func TestAPanicOnAConnectionKeepsTheSession(t *testing.T) {
	t.Parallel()

	local := freeLocalPort(t)
	dialer := panickingConnDialer{conn: panickingConn{fakeConn: newFakeConn()}}
	sess, err := runForwarder(dialer, "web", local, 80, make(chan struct{}), func() {}, time.Now)
	if err != nil {
		t.Fatalf("runForwarder: %v", err)
	}
	t.Cleanup(sess.Close)
	select {
	case <-sess.Ready:
	case err := <-sess.Err:
		t.Fatalf("Err before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("never ready")
	}

	client, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(local))))
	if err != nil {
		t.Fatalf("dial the forward: %v", err)
	}
	_ = client.Close()

	select {
	case <-sess.Errors:
	case <-time.After(5 * time.Second):
		t.Fatal("the session never recorded the panic")
	}
	// client-go closes the error stream before it reads it; either panic
	// may be the latest.
	if got := sess.LastError().Message; !strings.HasPrefix(got, ErrInternal.Error()+": ") || !strings.Contains(got, " bug") {
		t.Errorf("LastError = %q, want the panic", got)
	}
	select {
	case err := <-sess.Err:
		t.Errorf("the session ended: %v", err)
	default:
	}
}
