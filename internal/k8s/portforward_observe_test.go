package k8s

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	streamhttp "k8s.io/streaming/pkg/httpstream"
)

// refused is what a kubelet writes on the error stream when nothing
// listens on the remote port.
const refused = `error forwarding port 80 to pod 4c1f, uid : failed to execute portforward in network namespace "/var/run/netns/cni-1": failed to connect to localhost:80 inside namespace "4c1f", IPv4: dial tcp4 127.0.0.1:80: connect: connection refused IPv6 dial tcp6 [::1]:80: connect: connection refused `

// fakeStream is one stream of fakeConn. The error stream yields msg and
// ends; a data stream ends once it is reset or closed.
type fakeStream struct {
	headers http.Header
	id      uint32
	msg     []byte
	ended   chan struct{}
	once    sync.Once
}

func (s *fakeStream) Read(p []byte) (int, error) {
	if len(s.msg) > 0 {
		n := copy(p, s.msg)
		s.msg = s.msg[n:]
		return n, nil
	}
	if s.headers.Get(corev1.StreamType) == corev1.StreamTypeError {
		return 0, io.EOF
	}
	<-s.ended
	return 0, io.EOF
}

func (s *fakeStream) Write(p []byte) (int, error) { return len(p), nil }
func (s *fakeStream) Close() error                { s.end(); return nil }
func (s *fakeStream) Reset() error                { s.end(); return nil }
func (s *fakeStream) Headers() http.Header        { return s.headers }
func (s *fakeStream) Identifier() uint32          { return s.id }
func (s *fakeStream) end()                        { s.once.Do(func() { close(s.ended) }) }

// fakeConn is an upgraded connection to a pod whose remote port refuses
// connections. createErr, when set, fails every stream.
type fakeConn struct {
	createErr error

	mu      sync.Mutex
	next    uint32
	removed []uint32
	closed  chan bool
	once    sync.Once
}

func newFakeConn() *fakeConn { return &fakeConn{closed: make(chan bool)} }

func (c *fakeConn) CreateStream(h http.Header) (streamhttp.Stream, error) {
	if c.createErr != nil {
		return nil, c.createErr
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	s := &fakeStream{headers: h.Clone(), id: c.next, ended: make(chan struct{})}
	if h.Get(corev1.StreamType) == corev1.StreamTypeError {
		s.msg = []byte(refused)
	}
	return s, nil
}

func (c *fakeConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) CloseChan() <-chan bool         { return c.closed }
func (c *fakeConn) SetIdleTimeout(_ time.Duration) {}

func (c *fakeConn) RemoveStreams(streams ...streamhttp.Stream) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, s := range streams {
		c.removed = append(c.removed, s.Identifier())
	}
}

func (c *fakeConn) removedIDs() []uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint32(nil), c.removed...)
}

type fakeDialer struct{ conn *fakeConn }

func (d fakeDialer) Dial(_ ...string) (streamhttp.Connection, string, error) {
	return d.conn, "portforward.k8s.io", nil
}

func fixedClock(at time.Time) func() time.Time { return func() time.Time { return at } }

// TestATunnelErrorIsKeptOnTheSession runs client-go's forwarder over a
// connection whose pod refuses the remote port: the kubelet's message is
// kept on the session with its time, and the session says it changed.
func TestATunnelErrorIsKeptOnTheSession(t *testing.T) {
	t.Parallel()
	conn := newFakeConn()
	at := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	local := freeLocalPort(t)
	sess, err := runForwarder(fakeDialer{conn: conn}, "web", local, 80, make(chan struct{}), func() {}, fixedClock(at))
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
	if got := sess.LastError(); got != (TunnelError{}) {
		t.Fatalf("LastError before any connection = %+v", got)
	}

	client, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(local))))
	if err != nil {
		t.Fatalf("dial the forward: %v", err)
	}
	_ = client.Close()

	select {
	case <-sess.Errors:
	case <-time.After(5 * time.Second):
		t.Fatal("the session never said a tunnel error arrived")
	}
	if got := sess.LastError(); got.Message != strings.TrimSpace(refused) || !got.At.Equal(at) {
		t.Errorf("LastError = %+v, want the kubelet message at %v", got, at)
	}
	// client-go removes the streams it made; the observed error stream must
	// still be found by its id.
	deadline := time.Now().Add(5 * time.Second)
	for len(conn.removedIDs()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("removed streams = %v, want both", conn.removedIDs())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAFailedStreamIsATunnelError(t *testing.T) {
	t.Parallel()
	conn := newFakeConn()
	conn.createErr = errors.New("timed out waiting for the stream to be created")
	errs := newTunnelErrors(fixedClock(time.Unix(0, 0)))
	dialed, _, err := observe(fakeDialer{conn: conn}, errs).Dial("portforward.k8s.io")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dialed.CreateStream(http.Header{}); err == nil {
		t.Fatal("CreateStream succeeded")
	}
	if got := errs.last().Message; got != conn.createErr.Error() {
		t.Errorf("LastError = %q, want the stream error", got)
	}
}

func TestAnEmptyErrorStreamIsNoError(t *testing.T) {
	t.Parallel()
	errs := newTunnelErrors(time.Now)
	h := http.Header{}
	h.Set(corev1.StreamType, corev1.StreamTypeError)
	s := &observedStream{Stream: &fakeStream{headers: h, ended: make(chan struct{})}, errs: errs}
	if _, err := io.ReadAll(s); err != nil {
		t.Fatal(err)
	}
	if got := errs.last(); got != (TunnelError{}) {
		t.Errorf("LastError = %+v, want none", got)
	}
	select {
	case <-errs.changed:
		t.Error("an empty error stream said a tunnel error arrived")
	default:
	}
}
