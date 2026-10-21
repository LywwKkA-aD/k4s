package k8s

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// TunnelError is the latest failure on one of a running forward's
// connections, such as the kubelet's "connect: connection refused" when
// nothing listens on the remote port. The zero value means none.
type TunnelError struct {
	Message string
	At      time.Time
}

// maxTunnelError bounds how much of one error stream is kept.
const maxTunnelError = 1 << 10

// tunnelErrors keeps a session's latest TunnelError. client-go hands these
// failures only to the process-wide HandleError, which cannot tell one
// forward from another, so the session reads them off its own streams.
type tunnelErrors struct {
	now func() time.Time
	// changed holds one signal while a new error is unread; recording
	// never waits for a reader.
	changed chan struct{}

	mu     sync.Mutex
	latest TunnelError
}

func newTunnelErrors(now func() time.Time) *tunnelErrors {
	return &tunnelErrors{now: now, changed: make(chan struct{}, 1)}
}

func (e *tunnelErrors) record(msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	e.mu.Lock()
	e.latest = TunnelError{Message: msg, At: e.now()}
	e.mu.Unlock()
	select {
	case e.changed <- struct{}{}:
	default:
	}
}

func (e *tunnelErrors) last() TunnelError {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.latest
}

// observe wraps d so that every connection it dials reports its failures
// to errs: streams that cannot be created, and what the kubelet writes on
// a connection's error stream.
func observe(d streamhttp.Dialer, errs *tunnelErrors) streamhttp.Dialer {
	return observedDialer{Dialer: d, errs: errs}
}

type observedDialer struct {
	streamhttp.Dialer
	errs *tunnelErrors
}

// Dial implements streamhttp.Dialer.
func (d observedDialer) Dial(protocols ...string) (streamhttp.Connection, string, error) {
	conn, protocol, err := d.Dialer.Dial(protocols...)
	if err != nil {
		return conn, protocol, err
	}
	return observedConn{Connection: conn, errs: d.errs}, protocol, nil
}

type observedConn struct {
	streamhttp.Connection
	errs *tunnelErrors
}

// CreateStream implements streamhttp.Connection. client-go reads each
// connection's error stream to the end, so its whole message passes
// through observedStream.
func (c observedConn) CreateStream(headers http.Header) (streamhttp.Stream, error) {
	s, err := c.Connection.CreateStream(headers)
	if err != nil {
		c.errs.record(err.Error())
		return s, err
	}
	if headers.Get(corev1.StreamType) != corev1.StreamTypeError {
		return s, nil
	}
	return &observedStream{Stream: s, errs: c.errs}, nil
}

// observedStream is an error stream that records what it carried once it
// ends. One goroutine reads it.
type observedStream struct {
	streamhttp.Stream
	errs *tunnelErrors
	msg  []byte
}

// Read implements io.Reader.
func (s *observedStream) Read(p []byte) (int, error) {
	n, err := s.Stream.Read(p)
	if room := maxTunnelError - len(s.msg); room > 0 {
		s.msg = append(s.msg, p[:min(n, room)]...)
	}
	switch {
	case errors.Is(err, io.EOF):
		s.errs.record(string(s.msg))
	case err != nil:
		s.errs.record(err.Error())
	}
	return n, err
}
